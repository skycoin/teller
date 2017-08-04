package proxy

import "github.com/skycoin/teller/src/daemon"

// BindResponse http response for /api/bind
type BindResponse struct {
	BtcAddress string `json:"btc_address,omitempty"`
	Error      string `json:"error,omitempty"`
}

// StatusResponse http response for /api/status
type StatusResponse struct {
	Statuses []daemon.DepositStatus `json:"statuses,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

func makeBindHTTPResponse(rsp daemon.BindResponse) BindResponse {
	return BindResponse{
		BtcAddress: rsp.BtcAddress,
		Error:      rsp.Error,
	}
}

func makeStatusHTTPResponse(rsp daemon.StatusResponse) StatusResponse {
	return StatusResponse{
		Statuses: rsp.Statuses,
		Error:    rsp.Error,
	}
}
