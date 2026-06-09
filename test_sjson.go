package main

import (
	"fmt"
	"github.com/tidwall/sjson"
)

func main() {
	jsonc := `{
		// This is a comment
		"a": "salut",
		"config": {
			// another comment
			"c": 1
		}
	}`
	
	res, _ := sjson.Set(jsonc, "b", "autre")
	fmt.Println("After adding b:")
	fmt.Println(res)
	
	res2, _ := sjson.Set(res, "a", "monde")
	fmt.Println("After modifying a:")
	fmt.Println(res2)

	res3, _ := sjson.Set(res2, "config.d", 2)
	fmt.Println("After modifying config.d:")
	fmt.Println(res3)
}
