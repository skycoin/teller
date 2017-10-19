package monitor

import (
	"net/http"

	"github.com/rs/cors"
)

func LaunchServer(port string) {

	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir("../static/dist")))
	mux.HandleFunc("/addbtc", AddBtcAddresses)
	mux.HandleFunc("/getbtc", GetBtcAddresses)
	mux.HandleFunc("/getusedbtc", GetBtcUsedAddresses)
	mux.HandleFunc("/setusedbtc", SetUsedBtc)
	mux.HandleFunc("/allfreebtc", GetAllFreeBTC)
	mux.HandleFunc("/getcountbtc", GetCountBTC)

	handler := cors.Default().Handler(mux)
	http.ListenAndServe(":"+port, handler)
}
