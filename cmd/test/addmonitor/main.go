package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

func main() {
	now := time.Now()
	var success int
	var failed int
	n := 500
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		// wg.Add(1)
		// go func() {
		// 	defer wg.Done()
		s := fmt.Sprintf(`{
							"id": 0,
							"deposit_coin": {
								"coin": "BTC",
								"address": "abc%d"
							},
							"ico_coin": {
								"coin": "MZC",
								"address": "cBB6zToAjhMkvRrWaz8Tkv6QZxjKCfJWBx"
							}}`, rand.Int())
		req, err := http.NewRequest("POST", "http://localhost:7071/monitor", strings.NewReader(s))
		if err != nil {
			failed++
			fmt.Println(err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		cli := http.Client{}
		res, err := cli.Do(req)
		if err != nil {
			failed++
			fmt.Println(err)
			return
		}
		if res.StatusCode != 200 {
			fmt.Println(res.Status)
			failed++
			return
		}
		success++
		// }()
	}
	fmt.Println(n, "items, total spent:", time.Since(now), "success:", success, " failed:", failed)
	wg.Wait()
}
