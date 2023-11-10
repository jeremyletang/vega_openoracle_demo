vega_openoracle_demo
====================

This repository provide a small package to send prices feeds to the vega network using the open oracle format.
The code doing so is located under the `/prices` folder, and exposes a `Prices` type and a single public method named `Send`.

The requirement to use this package are:
- a vega wallet mnemonic
- an ethereum private key (hex formatted)
- the address of a vega node on the network with the grpc API enabled.


The Prices type will do the following:
- convert the price inputs into the open oracle using sign the payload, and return using the `code.vegaprotocol.io/oracles-relay/openoracle` package.
- a valid OracleSubmission transation is then built
- then emitted to the vega network using the wallet specified.

### Demo

A demonstration binary is available at the root. It can be installed using go 1.20.x.

```
$ go install github.com/jeremyletang/vega_openoracle_demo
```

A configuration is required to run it, here's an example:

```
node_addr = "n06.testnet.vega.xyz:3007"
ethereum_private_key = "0xd58575992709768084bf898fcafbb07d25746c566f53a75ee428708833677d2b"
wallet_mnemonic = "elder december include distance diary pizza bean churn fatigue repeat museum hello unfair march always loyal million video maple rich illegal industry borrow coral"
```

Assuming this configuration is save in a file name `config.json`, we could send price to vega with the following command:
```
$ vega_openoracle_demo -config=config.json -price=3354500 -timestamp=1699613159 -asset=BTC
```
