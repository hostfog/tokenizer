package tokenizer_test

import (
	"fmt"
	"log"

	"github.com/hostfog/tokenizer"
)

func Example() {
	tok, err := tokenizer.Load("testdata/gpt2.json")
	if err != nil {
		log.Fatal(err)
	}

	enc := tok.Encode("Hello world", true)
	fmt.Println(enc.IDs, enc.Tokens, enc.Offsets)
	fmt.Println(tok.Decode(enc.IDs, true))
	// Output:
	// [15496 995] [Hello Ġworld] [[0 5] [5 11]]
	// Hello world
}

func ExampleTokenizer_TokenToID() {
	tok, err := tokenizer.Load("testdata/gpt2.json")
	if err != nil {
		log.Fatal(err)
	}

	id, _ := tok.TokenToID("<|endoftext|>")
	fmt.Println(id, tok.VocabSize())
	// Output: 50256 50257
}
