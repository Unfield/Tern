# Native rendering spike

Go written by hand the way a compiler would generate it from the [vs-sveltekit](../vs-sveltekit) benchmark's `Shell` and `Items` components and Tern's `Root`. It answers two questions before building that compiler: is the HTML exactly Svelte's, and how much faster is it?

- **The HTML:** `TestSameHTMLAsSvelte` compares the output with what Svelte renders in QuickJS for `/items`. It is the same, byte for byte, hydration markers included.
- **JavaScript's rules:** templates depend on them, so `jsNumber` (`String(x)`) and `jsToFixed` follow JavaScript, checked against Node. For example `(0.125).toFixed(2)` is `"0.13"` where Go's `strconv` gives `0.12`, and `(-0.004).toFixed(2)` keeps its sign.

## Results

Apple M4, the 100-row `/items` page (22 KB of HTML):

| | Per render | Renders per second, 10 cores |
|---|---|---|
| Tern today: QuickJS | 2,918 µs | 688 |
| SvelteKit on Node, 1 and 10 processes | about 400 µs | 2,525 and 5,451 |
| **Native, from the load's Go structs** | **22 µs** | **about 156,000** |
| Native, decoding the JSON payload first | 102 µs | |

In Tern, a load's result is already Go values, so native rendering needs no decoding: the page's JSON payload is encoded anyway, for hydration.

## Running it

Capture the payload and QuickJS's body from the vs-sveltekit Tern app (`TestItemsDumpBody` in `internal/ssr` does it), then:

```sh
NATIVE_PAYLOAD=items-payload.json NATIVE_BODY=items-body.html go test -bench . -benchmem
```
