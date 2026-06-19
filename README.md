# Hashcash CLI Wallet

A small command-line wallet for Hashcash Credits (HCC). It uses the same
`~/.hccwallet.json` store as the Fyne GUI client and communicates with the
public Hashcash HTTPS API.

This client does not mine and does not create server-side accounts. Import an
existing HCC secret created by the web client, GUI client, or signup tool.

## Build

```bash
go build -o hcc-cli ./cmd/hcc-cli
```

Install it somewhere in `PATH`:

```bash
sudo install -m 0755 hcc-cli /usr/local/bin/hcc-cli
```

## Examples

```bash
hcc-cli importprivkey '<HCC_SECRET>' main
hcc-cli listwallets
hcc-cli setactivewallet main
hcc-cli getaddress
hcc-cli getbalance
hcc-cli sendtoaddress <HCC_ADDRESS> 10
hcc-cli listtransactions main 50
hcc-cli getaddressinfo <HCC_ADDRESS>
```

Select a wallet for one command:

```bash
hcc-cli --from savings getbalance
hcc-cli --from savings sendtoaddress <HCC_ADDRESS> 5
```

Use a different wallet file or API endpoint:

```bash
hcc-cli --wallet /path/to/wallet.json listwallets
hcc-cli --api https://hashcashfaucet.com/api getbalance
```

## Commands

- `getbalance [wallet]`
- `gettotalbalance`
- `getaddress [wallet]`
- `getwalletinfo [wallet]`
- `listwallets`
- `importprivkey <secret> [label]`
- `dumpprivkey [wallet]`
- `setactivewallet <wallet>`
- `renamewallet <wallet> <new-label>`
- `removewallet <wallet>`
- `sendtoaddress <address> <amount> [wallet]`
- `listtransactions [wallet] [limit]`
- `getaddressinfo <address>`

`wallet` can be a label or a 40-character HCC address.

## Security

The wallet store contains bearer secrets and is written with Unix permissions
`0600`. Anyone who obtains a secret can control that HCC account. Avoid passing
secrets on shared systems because shell history may retain them.
