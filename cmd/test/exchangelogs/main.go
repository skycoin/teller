package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
)

func main() {
	n := 100
	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			req, err := http.NewRequest("GET", "http://localhost:7071/exchange_logs?start=1&end=10", nil)
			if err != nil {
				fmt.Println(err)
				return
			}

			cli := http.Client{}
			res, err := cli.Do(req)
			if err != nil {
				fmt.Println(err)
				return
			}
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				fmt.Println(res.Status)
				v, err := ioutil.ReadAll(res.Body)
				if err != nil {
					fmt.Println(err)
					return
				}
				fmt.Println("body:", string(v))
				return
			}

			// var ack daemon.GetExchangeLogsAckMessage
			// if err := json.NewDecoder(res.Body).Decode(&ack); err != nil {
			// 	fmt.Println(err)
			// 	return
			// }
			fmt.Println("Success", i)
		}(i)
	}
	wg.Wait()
}
