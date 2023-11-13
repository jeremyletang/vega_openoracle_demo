package prices

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"code.vegaprotocol.io/oracles-relay/openoracle"
	"code.vegaprotocol.io/vega/commands"
	vgcrypto "code.vegaprotocol.io/vega/libs/crypto"
	apipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	commandspb "code.vegaprotocol.io/vega/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/vega/protos/vega/wallet/v1"
	wcommands "code.vegaprotocol.io/vega/wallet/commands"
	"code.vegaprotocol.io/vega/wallet/wallet"
	"github.com/ethereum/go-ethereum/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Prices struct {
	clt        apipb.CoreServiceClient
	w          *wallet.HDWallet
	firstKey   wallet.KeyPair
	ethPrivKey *ecdsa.PrivateKey
}

func New(
	vegaWalletMnemonic, ethPrivKey, vegaNodeAddr string,
) (*Prices, error) {
	// buidling the private from configuration
	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(ethPrivKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	publicKeyECDSA, ok := privKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	log.Printf("using wallet with address: %v", address)

	// instantiate a wallet from the mnemonic set in the configuration
	w, err := wallet.ImportHDWallet("oracleProvider", vegaWalletMnemonic, wallet.Version2)
	if err != nil {
		return nil, fmt.Errorf("could not import wallet: %v", err)
	}

	// generate the first key from the wallet.
	firstKey, err := w.GenerateKeyPair(nil)
	if err != nil {
		return nil, fmt.Errorf("could not generate first key: %v", err)
	}

	log.Printf("using vega wallet pubkey: %v", firstKey.PublicKey())

	// open a grpc client connection with the vega node,
	// this will be used to:
	// - pull the network latest block and chain ID (required to package the transaction
	// - send the transaction
	conn, err := grpc.Dial(vegaNodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("could not open connection with vega node: %v", err)
	}

	clt := apipb.NewCoreServiceClient(conn)

	return &Prices{
		clt:        clt,
		w:          w,
		firstKey:   firstKey,
		ethPrivKey: privKey,
	}, nil
}

// Send Prices will use the configured ethereum from the configuration to
// bundle the new prices, and sign the open oracles bundle.
// then the bundle is sent ot the vega network
func (p *Prices) Send(
	timestamp uint64,
	newPrices []openoracle.OraclePrice,
) error {

	// make the open oracle payload
	req := openoracle.OracleRequest{
		Timestamp: timestamp,
		Prices:    newPrices,
	}

	// this uses the private keys to sign the bundle,
	// return the OpenOracleRequest type, which is later
	// marshall to json and sent to vega
	res, err := req.IntoOpenOracle(p.ethPrivKey)
	if err != nil {
		return fmt.Errorf("unable to build open oracle payload: %v", err)
	}

	log.Printf("encode open oracle message %#v\n", res)

	log.Printf("verifying the payload")

	// verify the validaty of the signatures for conveniences
	address, prices, err := openoracle.Verify(*res)
	if err != nil {
		return fmt.Errorf("enable to verify open oracle payload: %w", err)
	}

	log.Printf("recovered ethereum address from the signatures: %v", address)
	log.Printf("recovered prices: %#v", prices)

	// send the transaction to the vega network
	return p.sendToVegaNetwork(res)
}

func (p *Prices) sendToVegaNetwork(res *openoracle.OracleResponse) error {
	// marshall the open oracle payload to json
	bytes, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("could not marshal transaction to json: %v", err)
	}

	// package the vega transaction input data
	cmd := &commandspb.OracleDataSubmission{
		Source:  commandspb.OracleDataSubmission_ORACLE_SOURCE_OPEN_ORACLE,
		Payload: bytes,
	}

	// Bundle our OracleDataSubmission command
	tx, err := p.bundleTransaction(cmd)
	if err != nil {
		return err
	}

	log.Printf("packaged transaction: %#v\n", tx)

	// Submit it to the vega network
	sres, err := p.clt.SubmitTransaction(context.Background(), &apipb.SubmitTransactionRequest{
		Tx:   tx,
		Type: apipb.SubmitTransactionRequest_TYPE_SYNC,
	})
	if err != nil {
		return fmt.Errorf("could not submit transaction: %v", err)
	}

	log.Printf("transaction result: success(%v), hash(%v), err(%v)", sres.Success, sres.TxHash, sres.Data)

	return nil
}

// this use the client and command to bundle a new transaction to the network
func (p *Prices) bundleTransaction(
	cmd *commandspb.OracleDataSubmission,
) (*commandspb.Transaction, error) {

	// pull the block data from the netowkr, use to sign the transaction and build the pow challenge
	lastBlockData, err := p.clt.LastBlockHeight(context.Background(), &apipb.LastBlockHeightRequest{})
	if err != nil {
		return nil, fmt.Errorf("could not get last block height: %v", err)
	}

	// marshal the input data to be injected in the transaction
	marshaledInputData, err := wcommands.ToMarshaledInputData(
		&walletpb.SubmitTransactionRequest{
			PubKey: p.firstKey.PublicKey(),
			Command: &walletpb.SubmitTransactionRequest_OracleDataSubmission{
				OracleDataSubmission: cmd,
			},
		},
		lastBlockData.Height,
	)
	if err != nil {
		return nil, fmt.Errorf("could not marshal the input data: %v", err)
	}

	// Sign the transaction
	signature, err := p.w.SignTx(p.firstKey.PublicKey(), commands.BundleInputDataForSigning(marshaledInputData, lastBlockData.ChainId))
	if err != nil {
		return nil, fmt.Errorf("could not sign the transaction: %v", err)
	}

	// package the signatures
	tx := commands.NewTransaction(p.firstKey.PublicKey(), marshaledInputData, &commandspb.Signature{
		Value:   signature.Value,
		Algo:    signature.Algo,
		Version: signature.Version,
	})

	// Generate the proof of work for the transaction.
	txID := vgcrypto.RandomHash()
	powNonce, _, err := vgcrypto.PoW(lastBlockData.Hash, txID, uint(lastBlockData.SpamPowDifficulty), lastBlockData.SpamPowHashFunction)
	if err != nil {
		return nil, fmt.Errorf("could not compute the proof-of-work: %v", err)
	}

	tx.Pow = &commandspb.ProofOfWork{
		Nonce: powNonce,
		Tid:   txID,
	}

	return tx, nil
}
