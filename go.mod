module github.com/jeremyletang/vega_openoracle_demo

go 1.20

require (
	code.vegaprotocol.io/oracles-relay v0.0.0-20220819135656-783260e20264
	code.vegaprotocol.io/vega v0.73.3
	github.com/ethereum/go-ethereum v1.11.6
	github.com/pelletier/go-toml v1.9.5
	google.golang.org/grpc v1.56.3
)

require (
	github.com/PaesslerAG/gval v1.0.0 // indirect
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.9.0 // indirect
	github.com/holiman/uint256 v1.2.2-0.20230321075855-87b91420868c // indirect
	github.com/oasisprotocol/curve25519-voi v0.0.0-20220317090546-adb2f9614b17 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/tyler-smith/go-bip39 v1.1.0 // indirect
	github.com/vegaprotocol/go-slip10 v0.1.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.24.0 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)

replace code.vegaprotocol.io/oracles-relay => ../oracles-relay
replace github.com/shopspring/decimal => github.com/vegaprotocol/decimal v1.3.1-uint256