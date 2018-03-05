package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Do does request to given addr and endpoint
func Do(addr, endpoint string, r Request) (*Response, error) {
	c := http.Client{}
	requestURI := url.URL{}

	requestURI.Host = addr
	requestURI.Scheme = "http"
	requestURI.Path = "/" + endpoint
	requestData, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", requestURI.String(), bytes.NewReader(requestData))
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			panic(err)
		}
	}()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("reesponse status code %d", resp.StatusCode)
	}
	respdata, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var rpcResp Response
	err = json.Unmarshal(respdata, &rpcResp)
	if err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	return &rpcResp, nil

}
