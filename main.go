package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"math/big"
	"os"
	"strings"

	"code.vegaprotocol.io/oracles-relay/coinbase"
	"code.vegaprotocol.io/oracles-relay/openoracle"
	"code.vegaprotocol.io/vega/commands"
	vgcrypto "code.vegaprotocol.io/vega/libs/crypto"
	apipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
	wcommands "code.vegaprotocol.io/vega/wallet/commands"
	"code.vegaprotocol.io/vega/wallet/wallet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jeremyletang/vega_openoracle_demo/prices"
	"github.com/pelletier/go-toml"
)

type Config struct {
	NodeAddr string `toml:"node_addr"`

	WalletMnemonic string `toml:"wallet_mnemonic"`

	EthereumPrivateKey string `toml:"ethereum_private_key"`
	// The coinbase config is not mandatory
	// if nil, we do not start the worker
	Coinbase *coinbase.Config `toml:"coinbase"`
}

var flags = struct {
	Config   string
	Coinbase bool
	Local    bool

	Price     string
	Timestamp uint64
	Asset     string
}{}

func init() {
	flag.StringVar(&flags.Config, "config", "config.toml", "The configuration of the oracle relay")
	flag.BoolVar(&flags.Coinbase, "coinbase", false, "Use coinbase data")
	flag.BoolVar(&flags.Local, "local", false, "Use command line specified data")

	flag.StringVar(&flags.Price, "price", "", "The price to encode (optional)")
	flag.StringVar(&flags.Asset, "asset", "", "The asset name (optional)")
	flag.Uint64Var(&flags.Timestamp, "timestamp", 0, "The timestamp at which to publish the data")
}

func main() {
	flag.Parse()

	if flags.Coinbase && flags.Local {
		log.Fatalf("cannot use both flags coinbase and local")
	}

	// load our configuration
	config, err := loadConfig(flags.Config)
	if err != nil {
		log.Fatalf("unable to read configuration: %v", err)
	}

	// validate common config
	if len(config.EthereumPrivateKey) <= 0 {
		log.Fatalf("missing ethereum private key")
	}

	if len(config.NodeAddr) <= 0 {
		log.Fatalf("missing node address")
	}

	if len(config.WalletMnemonic) <= 0 {
		log.Fatalf("missing wallet mnemonic")
	}

	if flags.Coinbase {
		runCoinbaseExample(*config)
		return
	}

	runLocalExample(*config)
}

func runLocalExample(config Config) {
	if len(flags.Price) <= 0 {
		log.Fatalf("missing price")
	}

	price, _ := big.NewInt(0).SetString(flags.Price, 10)
	if price == nil {
		log.Fatalf("invalid price: %v", flags.Price)
	}

	if len(flags.Asset) <= 0 {
		log.Fatalf("missing asset")
	}

	if flags.Timestamp == 0 {
		log.Fatalf("missing timestamp")
	}

	p, err := prices.New(
		config.WalletMnemonic,
		config.EthereumPrivateKey,
		config.NodeAddr,
	)
	if err != nil {
		log.Fatalf("could not instanties prices: %v", err)
	}

	err = p.Send(
		flags.Timestamp,
		[]openoracle.OraclePrice{
			{
				Asset:     flags.Asset,
				Timestamp: flags.Timestamp,
				Price:     flags.Price,
			},
		},
	)
	if err != nil {
		log.Fatalf("could not send the prices: %v", err)
	}
}

func runCoinbaseExample(config Config) {
	if config.Coinbase == nil {
		log.Fatalf("missing coinbase config")
	}

	cb := coinbase.New(*config.Coinbase)

	bytes, err := cb.Pull()
	if err != nil {
		log.Fatalf("could not load coinbase open oracle: %v", err)
	}

	log.Printf("receive open oracle data%v\n", string(bytes))

	res, err := openoracle.Unmarshal(bytes)
	if err != nil {
		log.Fatalf("unable to unmarshal coinbase open oracle data")
	}

	sendToVegaNetwork(config, res)
}

// Send Prices will use the configured ethereum from the configuration to
// bundle the new prices, and sign the open oracles bundle.
// then the bundle is sent ot the vega network
func SendPrices(config Config, timestamp uint64, newPrices []openoracle.OraclePrice) {
	// buidling the private from configuration
	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(config.EthereumPrivateKey, "0x"))
	if err != nil {
		log.Fatalf("invalid private key")
	}

	// make the open oracle payload
	req := openoracle.OracleRequest{
		Timestamp: timestamp,
		Prices:    newPrices,
	}

	// this uses the private keys to sign the bundle,
	// return the OpenOracleRequest type, which is later
	// marshall to json and sent to vega
	res, err := req.IntoOpenOracle(privKey)
	if err != nil {
		log.Fatalf("unable to build open oracle payload: %v", err)
	}

	log.Printf("encode open oracle message %#v\n", res)

	log.Printf("verifying the payload")

	// verify the validaty of the signatures for conveniences
	address, prices, err := openoracle.Verify(*res)
	if err != nil {
		log.Fatalf("enable to verify open oracle payload: %v", err)
	}

	log.Printf("recovered ethereum address from the signatures: %v", address)
	log.Printf("recovered prices: %#v", prices)

	// send the transaction to the vega network
	sendToVegaNetwork(config, res)
}

func sendToVegaNetwork(config Config, res *openoracle.OracleResponse) {
	// marshall the open oracle payload to json
	bytes, err := json.Marshal(res)
	if err != nil {
		log.Fatalf("could not marshal transaction to json: %v", err)
	}

	// package the vega transaction input data
	cmd := &commandspb.OracleDataSubmission{
		Source:  commandspb.OracleDataSubmission_ORACLE_SOURCE_OPEN_ORACLE,
		Payload: bytes,
	}

	// open a grpc client connection with the vega node,
	// this will be used to:
	// - pull the network latest block and chain ID (required to package the transaction
	// - send the transaction
	conn, err := grpc.Dial(config.NodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not open connection with vega node: %v", err)
	}

	clt := apipb.NewCoreServiceClient(conn)

	// Bundle our OracleDataSubmission command
	tx := bundleTransaction(config, cmd, clt)

	log.Printf("packaged transaction: %#v\n", tx)

	// Submit it to the vega network
	sres, err := clt.SubmitTransaction(context.Background(), &apipb.SubmitTransactionRequest{
		Tx:   tx,
		Type: apipb.SubmitTransactionRequest_TYPE_SYNC,
	})
	if err != nil {
		log.Fatalf("could not submit transaction: %v", err)
	}

	log.Printf("transaction result: success(%v), hash(%v), err(%v)", sres.Success, sres.TxHash, sres.Data)
}

// this use the client and command to bundle a new transaction to the network
func bundleTransaction(config Config, cmd *commandspb.OracleDataSubmission, clt apipb.CoreServiceClient) *commandspb.Transaction {
	// instantiate a wallet from the mnemonic set in the configuration
	w, err := wallet.ImportHDWallet("oracleProvider", config.WalletMnemonic, wallet.Version2)
	if err != nil {
		log.Fatalf("could not import wallet: %v", err)
	}

	// generate the first key from the wallet.
	firstKey, err := w.GenerateKeyPair(nil)
	if err != nil {
		log.Fatalf("could not generate first key: %v", err)
	}

	// pull the block data from the netowkr, use to sign the transaction and build the pow challenge
	lastBlockData, err := clt.LastBlockHeight(context.Background(), &apipb.LastBlockHeightRequest{})
	if err != nil {
		log.Fatalf("could not get last block height: %v", err)
	}

	// marshal the input data to be injected in the transaction
	marshaledInputData, err := wcommands.ToMarshaledInputData(
		&walletpb.SubmitTransactionRequest{
			PubKey: firstKey.PublicKey(),
			Command: &walletpb.SubmitTransactionRequest_OracleDataSubmission{
				OracleDataSubmission: cmd,
			},
		},
		lastBlockData.Height,
	)
	if err != nil {
		log.Fatalf("could not marshal the input data: %v", err)
	}

	// Sign the transaction
	signature, err := w.SignTx(firstKey.PublicKey(), commands.BundleInputDataForSigning(marshaledInputData, lastBlockData.ChainId))
	if err != nil {
		log.Fatalf("could not sign the transaction: %v", err)
	}

	tx := commands.NewTransaction(firstKey.PublicKey(), marshaledInputData, &commandspb.Signature{
		Value:   signature.Value,
		Algo:    signature.Algo,
		Version: signature.Version,
	})

	// Generate the proof of work for the transaction.
	txID := vgcrypto.RandomHash()
	powNonce, _, err := vgcrypto.PoW(lastBlockData.Hash, txID, uint(lastBlockData.SpamPowDifficulty), lastBlockData.SpamPowHashFunction)
	if err != nil {
		log.Fatalf("could not compute the proof-of-work: %v", err)
	}

	tx.Pow = &commandspb.ProofOfWork{
		Nonce: powNonce,
		Tid:   txID,
	}

	return tx
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := Config{}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
