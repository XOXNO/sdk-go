# XOXNO Go SDK

A small Stellar swap client. Quotes and prepares unsigned XOXNO transactions; it never signs or submits them.

```go
client := xoxno.NewClient(xoxno.Config{
    QuoteURL: "https://stellar-swap.xoxno.com",
    Router: routerContract,
    NetworkPassphrase: network.PublicNetworkPassphrase,
})
quote, err := client.Quote(ctx, xoxno.QuoteRequest{
    SourceToken: inputContract, DestToken: outputContract,
    SourceAmount: "10", SourceDecimals: 7, DestDecimals: 7,
    Sender: accountAddress, SlippagePercent: 1,
    PrepareTransaction: true, AccountSequence: accountSequence,
    TimeoutSeconds: 180,
})
```

Contract identifiers and decimals come from the caller's canonical asset registry. Account lookup, destination trustline checks, provider competition and application caching remain with the caller. Set `PrepareTransaction: false` to request a quote without preparing a transaction; this says nothing about the account's trustlines. `QuoteInput` sizes input for a desired output; obtain a fresh forward quote before signing.

The SDK checks quote identity, exact amounts, slippage, decimals, simulation, pinned router/network and envelope source/operations/fees/authorization. It changes only sequence and expiry after validation. Final envelope metadata includes total fee, resource fee and expiry. Configure the router independently of upstream responses.

`ListedTokens`, `Tokens` and `Prices` fetch provider metadata without caching. HTTP responses are bounded; all requests accept cancellation through context.

`ReadReceipt` accepts a confirmed envelope, result and metadata. It recognizes a selected XOXNO operation, reads router-to-viewer output transfers, and binds their event preimage to the successful operation result. A nil receipt means unavailable or unsupported, never a zero output. V3 metadata requires a single operation; V4 uses the selected operation's events and requires the return value. Core metadata with added SAC reconciliation events is currently treated as unavailable rather than reported as confirmed. The caller must verify the transaction hash and network when obtaining the envelope/result from its configured ledger service.

## Checks

```sh
go test -race ./...
go vet ./...
```

Envelope validation originated in the XOXNO swap integration for `stellar/freighter-backend-v2`.
