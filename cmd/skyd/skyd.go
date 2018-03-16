// a local fake skyd that reports deposits to teller for testing
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flag"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/skycoin/skycoin/src/visor"
	"github.com/skycoin/skycoin/src/coin"
	"crypto/rand"
	"github.com/skycoin/skycoin/src/cipher"
	"log"
)

const (
	rpcQuirks = true

	// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	// The timeout configuration is necessary for public servers, or else
	// connections will be used up
	serverReadTimeout  = time.Second * 10
	serverWriteTimeout = time.Second * 20
	serverIdleTimeout  = time.Second * 120
)

// Deposit records information about a BTC deposit
type Deposit struct {
	Address string // deposit address
	Value   int64  // deposit amount. For BTC, measured in satoshis.
	Height  int64  // the block height
	Tx      string // the transaction id
	N       uint32 // the index of vout in the tx [BTC]
}

// BlockStore holds fake block data
type BlockStore struct {
	sync.RWMutex
	BestBlockHeight int32
	BlockHashes     map[int64]string
	HashBlocks      map[string]visor.ReadableBlocks
}

var initialBlock visor.ReadableBlocks

var defaultBlockStore *BlockStore

type commandHandler func(*rpcServer, interface{}, <-chan struct{}) (interface{}, error)

var rpcHandlers = map[string]commandHandler{
	"get_blocks":        handleGetBlock,
	"get_blocks_by_seq": handleGetBestBlock,
	"get_lastblocks":    handleGetBlockHash,
	//"getblockcount": handleGetBlockCount,
	"nextdeposit": handleNextDeposit, // for triggering a fake deposit
}

type rpcServer struct {
	started                int32
	shutdown               int32
	listeners              []net.Listener
	wg                     sync.WaitGroup
	requestProcessShutdown chan struct{}
	key                    string
	cert                   string
	address                string
	maxConcurrentReqs      int
}

func handleGetBestBlock(s *rpcServer, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	if hash, ok := defaultBlockStore.BlockHashes[int64(defaultBlockStore.BestBlockHeight)]; ok {
		result := &btcjson.GetBestBlockResult{
			Hash:   hash,
			Height: defaultBlockStore.BestBlockHeight,
		}
		return result, nil
	}
	return nil, &btcjson.RPCError{
		Code:    btcjson.ErrRPCBlockNotFound,
		Message: "Block not found",
	}
}

func handleGetBlock(s *rpcServer, cmd interface{}, closeChan <-chan struct{}) (retBlock interface{}, err error) {
	c := cmd.(*btcjson.GetBlockCmd)
	if block, ok := defaultBlockStore.HashBlocks[c.Hash]; ok {
		//block.NextHash = ""
		//if block.Height < int64(defaultBlockStore.BestBlockHeight) {
		//	if hash, ok := defaultBlockStore.BlockHashes[block.Height+1]; ok {
		//		block.NextHash = hash
		//	}
		//}

		retBlock = block

		return
	}

	return nil, &btcjson.RPCError{
		Code:    btcjson.ErrRPCBlockNotFound,
		Message: "Block not found",
	}
}

func handleGetBlockHash(s *rpcServer, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	c := cmd.(*btcjson.GetBlockHashCmd)
	if hash, ok := defaultBlockStore.BlockHashes[c.Index]; ok {
		return hash, nil
	}

	return nil, &btcjson.RPCError{
		Code:    btcjson.ErrRPCBlockNotFound,
		Message: "Block not found",
	}
}

func handleGetBlockCount(s *rpcServer, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	return int64(defaultBlockStore.BestBlockHeight), nil
}

func processDeposits(deposits []Deposit) (blocks visor.ReadableBlocks, err error) {
	defaultBlockStore.Lock()
	defer defaultBlockStore.Unlock()

	//bestHeight := int64(defaultBlockStore.BestBlockHeight)

	//// Add new block
	value := strconv.Itoa(int(deposits[0].Value))
	blocks = createFakeBlock(deposits[0].Address, value)
	defaultBlockStore.BestBlockHeight++

	hash := blocks.Blocks[0].Head.BodyHash

	//// Update NextHash of previous block
	//prevBlockHash := defaultBlockStore.BlockHashes[bestHeight]
	//prevBlock := defaultBlockStore.HashBlocks[prevBlockHash]
	//prevBlock.NextHash = block.Hash().String()
	//defaultBlockStore.HashBlocks[prevBlockHash] = prevBlock
	//
	// Add new block
	defaultBlockStore.BestBlockHeight++
	defaultBlockStore.BlockHashes[int64(defaultBlockStore.BestBlockHeight)] = hash
	defaultBlockStore.HashBlocks[hash] = blocks

	return
}

func createFakeBlock(address, coins string) (blocks visor.ReadableBlocks) {

	if address == "" {
		address = "cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW"
	}
	if coins == "" {
		coins = "1"
	}

	blocks = visor.ReadableBlocks{
		Blocks: []visor.ReadableBlock{
			{
				Head: visor.ReadableBlockHeader{
					BkSeq:             1,
					BlockHash:         "662835cc081e037561e1fe05860fdc4b426f6be562565bfaa8ec91be5675064a",
					PreviousBlockHash: "f680fe1f068a1cd5c3ef9194f91a9bc3cacffbcae4a32359a3c014da4ef7516f",
					Time:              1,
					Fee:               20732,
					Version:           1,
					BodyHash:          "tx_body_hash",
				},
				Body: visor.ReadableBlockBody{
					Transactions: []visor.ReadableTransaction{
						{
							Length:    608,
							Type:      0,
							Hash:      "662835cc081e037561e1fe05860fdc4b426f6be562565bfaa8ec91be5675064a",
							InnerHash: "37f1111bd83d9c995b9e48511bd52de3b0e440dccbf6d2cfd41dee31a10f1aa4",
							Timestamp: 1,
							Sigs: []string{
								"ef0b8e1465557e6f21cb2bfad17136188f0b9bd54bba3db76c3488eb8bc900bc7662e3fe162dd6c236d9e52a7051a2133855081a91f6c1a63e1fce2ae9e3820e00",
								"800323c8c22a2c078cecdfad35210902f91af6f97f0c63fe324e0a9c2159e9356f2fbbfff589edea5a5c24453ef5fc0cd5929f24bebee28e37057acd6d42f3d700",
								"ca6a6ef5f5fb67490d88ddeeee5e5d11055246613b03e7ed2ad5cc82d01077d262e2da56560083928f5389580ae29500644719cf0e82a5bf065cecbed857598400",
								"78ddc117607159c7b4c76fc91deace72425f21f2df5918d44d19a377da68cc610668c335c84e2bb7a8f16cd4f9431e900585fc0a3f1024b722b974fcef59dfd500",
								"4c484d44072e23e97a437deb03a85e3f6eca0bd8875031efe833e3c700fc17f91491969b9864b56c280ef8a68d18dd728b211ce1d46fe477fe3104d73d55ad6501",
							},
							In: []string{
								"4bd7c68ecf3039c2b2d8c26a5e2983e20cf53b6d62b099e7786546b3c3f600f9",
								"f9e39908677cae43832e1ead2514e01eaae48c9a3614a97970f381187ee6c4b1",
								"7e8ac23a2422b4666ff45192fe36b1bd05f1285cf74e077ac92cabf5a7c1100e",
								"b3606a4f115d4161e1c8206f4fb5ac0e91551c40d0ee6fe40c86040d2faacac0",
								"305f1983f5b630bba27e2777c229c725b6b57f37a6ddee138d1d82ae56311909",
							},
							Out: []visor.ReadableTransactionOutput{
								{
									Hash:    "574d7e5afaefe4ee7e0adf6ce1971d979f038adc8ebbd35771b2c19b0bad7e3d",
									Address: address,
									Coins:   coins,
									Hours:   3455,
								},
							},
						},
					},
				},
			},

		},
	}

	return
}

func handleNextDeposit(s *rpcServer, cmd interface{}, closeChan <-chan struct{}) (interface{}, error) {
	var deposits []Deposit
	if cmd != nil {
		deposits = cmd.([]Deposit)
		fmt.Printf("Got %v\n", deposits)
	}

	newBlock, err := processDeposits(deposits)

	if err != nil {
		fmt.Printf("processDeposits %v\n", err)
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCBlockNotFound,
			Message: "Block not found",
		}
	}

	return newBlock, nil
}

// parseListeners determines whether each listen address is IPv4 and IPv6 and
// returns a slice of appropriate net.Addrs to listen on with TCP. It also
// properly detects addresses which apply to "all interfaces" and adds the
// address as both IPv4 and IPv6.
func parseListeners(addrs []string) ([]net.Addr, error) {
	netAddrs := make([]net.Addr, 0, len(addrs)*2)
	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			// Shouldn't happen due to already being normalized.
			return nil, err
		}

		// Empty host or host of * on plan9 is both IPv4 and IPv6.
		if host == "" || (host == "*" && runtime.GOOS == "plan9") {
			netAddrs = append(netAddrs, simpleAddr{net: "tcp4", addr: addr})
			netAddrs = append(netAddrs, simpleAddr{net: "tcp6", addr: addr})
			continue
		}

		// Strip IPv6 zone id if present since net.ParseIP does not
		// handle it.
		zoneIndex := strings.LastIndex(host, "%")
		if zoneIndex > 0 {
			host = host[:zoneIndex]
		}

		// Parse the IP.
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("'%s' is not a valid IP address", host)
		}

		// To4 returns nil when the IP is not an IPv4 address, so use
		// this determine the address type.
		if ip.To4() == nil {
			netAddrs = append(netAddrs, simpleAddr{net: "tcp6", addr: addr})
		} else {
			netAddrs = append(netAddrs, simpleAddr{net: "tcp4", addr: addr})
		}
	}
	return netAddrs, nil
}

func setupRPCListeners(key string, cert string, address string) ([]net.Listener, error) {

	// Change the standard net.Listen function to the tls one.

	netAddrs, err := parseListeners([]string{address})
	if err != nil {
		return nil, err
	}

	listeners := make([]net.Listener, 0, len(netAddrs))
	for _, addr := range netAddrs {
		listener, err := net.Listen(addr.Network(), addr.String())
		if err != nil {
			fmt.Printf("Can't listen on %s: %v\n", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}

	return listeners, nil
}

// parsedRPCCmd represents a JSON-RPC request object that has been parsed into
// a known concrete command along with any error that might have happened while
// parsing it.
type parsedRPCCmd struct {
	id     interface{}
	method string
	cmd    interface{}
	err    *btcjson.RPCError
}

// parseCmd parses a JSON-RPC request object into known concrete command.  The
// err field of the returned parsedRPCCmd struct will contain an RPC error that
// is suitable for use in replies if the command is invalid in some way such as
// an unregistered command or invalid parameters.
func parseCmd(request *btcjson.Request) *parsedRPCCmd {
	var parsedCmd parsedRPCCmd
	parsedCmd.id = request.ID
	parsedCmd.method = request.Method

	cmd, err := btcjson.UnmarshalCmd(request)

	// Handle new commands except btcd cmds
	if request.Method == "nextdeposit" {
		if len(request.Params) == 1 {
			var deposit []Deposit
			err := json.Unmarshal(request.Params[0], &deposit)
			if err != nil {
				parsedCmd.err = btcjson.ErrRPCMethodNotFound
				return &parsedCmd
			}
			fmt.Printf("%v\n", deposit)
			parsedCmd.cmd = deposit
		}
		return &parsedCmd
	}

	if err != nil {
		// When the error is because the method is not registered,
		// produce a method not found RPC error.
		if jerr, ok := err.(btcjson.Error); ok &&
			jerr.ErrorCode == btcjson.ErrUnregisteredMethod {

			parsedCmd.err = btcjson.ErrRPCMethodNotFound
			return &parsedCmd
		}

		// Otherwise, some type of invalid parameters is the
		// cause, so produce the equivalent RPC error.
		parsedCmd.err = btcjson.NewRPCError(
			btcjson.ErrRPCInvalidParams.Code, err.Error())
		return &parsedCmd
	}

	parsedCmd.cmd = cmd
	return &parsedCmd
}

// standardCmdResult checks that a parsed command is a standard Bitcoin JSON-RPC
// command and runs the appropriate handler to reply to the command.  Any
// commands which are not recognized or not implemented will return an error
// suitable for use in replies.
func (s *rpcServer) standardCmdResult(cmd *parsedRPCCmd, closeChan <-chan struct{}) (interface{}, error) {
	handler, ok := rpcHandlers[cmd.method]
	if ok {
		return handler(s, cmd.cmd, closeChan)
	}
	return nil, btcjson.ErrRPCMethodNotFound
}

// createMarshalledReply returns a new marshalled JSON-RPC response given the
// passed parameters.  It will automatically convert errors that are not of
// the type *btcjson.RPCError to the appropriate type as needed.
func createMarshalledReply(id, result interface{}, replyErr error) ([]byte, error) {
	var jsonErr *btcjson.RPCError
	if replyErr != nil {
		if jErr, ok := replyErr.(*btcjson.RPCError); ok {
			jsonErr = jErr
		} else {
			jsonErr = btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, replyErr.Error())
		}
	}

	return btcjson.MarshalResponse(id, result, jsonErr)
}

// httpStatusLine returns a response Status-Line (RFC 2616 Section 6.1)
// for the given request and response status code.  This function was lifted and
// adapted from the standard library HTTP server code since it's not exported.
func (s *rpcServer) httpStatusLine(req *http.Request) string {
	code := http.StatusOK
	proto11 := req.ProtoAtLeast(1, 1)

	proto := "HTTP/1.0"
	if proto11 {
		proto = "HTTP/1.1"
	}
	codeStr := strconv.Itoa(code)
	text := http.StatusText(code)
	if text != "" {
		return proto + " " + codeStr + " " + text + "\r\n"
	}

	text = "status code " + codeStr
	return proto + " " + codeStr + " " + text + "\r\n"
}

// writeHTTPResponseHeaders writes the necessary response headers prior to
// writing an HTTP body given a request to use for protocol negotiation, headers
// to write, and a writer.
func (s *rpcServer) writeHTTPResponseHeaders(req *http.Request, headers http.Header, w io.Writer) error {
	_, err := io.WriteString(w, s.httpStatusLine(req))
	if err != nil {
		return err
	}

	err = headers.Write(w)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, "\r\n")
	return err
}

// jsonRPCRead handles reading and responding to RPC messages.
func (s *rpcServer) jsonRPCRead(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}

	// Read and close the JSON-RPC request body from the caller.
	body, err := ioutil.ReadAll(r.Body)
	if err := r.Body.Close(); err != nil {
		fmt.Println("Failed to close response body:", err)
	}

	if err != nil {
		errCode := http.StatusBadRequest
		http.Error(w, fmt.Sprintf("%d error reading JSON message: %v",
			errCode, err), errCode)
		return
	}

	// Unfortunately, the http server doesn't provide the ability to
	// change the read deadline for the new connection and having one breaks
	// long polling.  However, not having a read deadline on the initial
	// connection would mean clients can connect and idle forever.  Thus,
	// hijack the connecton from the HTTP server, clear the read deadline,
	// and handle writing the response manually.
	hj, ok := w.(http.Hijacker)
	if !ok {
		errMsg := "webserver doesn't support hijacking"
		fmt.Print(errMsg)
		errCode := http.StatusInternalServerError
		http.Error(w, strconv.Itoa(errCode)+" "+errMsg, errCode)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		fmt.Printf("Failed to hijack HTTP connection: %v", err)
		errCode := http.StatusInternalServerError
		http.Error(w, strconv.Itoa(errCode)+" "+err.Error(), errCode)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Println("conn.Close failed:", err)
		}
	}()
	defer func() {
		if err := buf.Flush(); err != nil {
			fmt.Println("buf.Flush failed:", err)
		}
	}()
	if err := conn.SetReadDeadline(timeZeroVal); err != nil {
		fmt.Println("conn.SetReadDeadline failed:", err)
	}

	// Attempt to parse the raw body into a JSON-RPC request.
	var responseID interface{}
	var jsonErr error
	var result interface{}
	var request btcjson.Request
	if err := json.Unmarshal(body, &request); err != nil {
		jsonErr = &btcjson.RPCError{
			Code:    btcjson.ErrRPCParse.Code,
			Message: "Failed to parse request: " + err.Error(),
		}
	}
	if jsonErr == nil {
		// The JSON-RPC 1.0 spec defines that notifications must have their "id"
		// set to null and states that notifications do not have a response.
		//
		// A JSON-RPC 2.0 notification is a request with "json-rpc":"2.0", and
		// without an "id" member. The specification states that notifications
		// must not be responded to. JSON-RPC 2.0 permits the null value as a
		// valid request id, therefore such requests are not notifications.
		//
		// Bitcoin Core serves requests with "id":null or even an absent "id",
		// and responds to such requests with "id":null in the response.
		//
		// Btcd does not respond to any request without and "id" or "id":null,
		// regardless the indicated JSON-RPC protocol version unless RPC quirks
		// are enabled. With RPC quirks enabled, such requests will be responded
		// to if the reqeust does not indicate JSON-RPC version.
		//
		// RPC quirks can be enabled by the user to avoid compatibility issues
		// with software relying on Core's behavior.
		if request.ID == nil && !(rpcQuirks && request.Jsonrpc == "") {
			return
		}

		// The parse was at least successful enough to have an ID so
		// set it for the response.
		responseID = request.ID

		// Setup a close notifier.  Since the connection is hijacked,
		// the CloseNotifer on the ResponseWriter is not available.
		closeChan := make(chan struct{}, 1)
		go func() {
			_, err := conn.Read(make([]byte, 1))
			if err != nil {
				close(closeChan)
			}
		}()

		if jsonErr == nil {
			// Attempt to parse the JSON-RPC request into a known concrete
			// command.
			parsedCmd := parseCmd(&request)

			if parsedCmd.err != nil {
				jsonErr = parsedCmd.err
			} else {
				result, jsonErr = s.standardCmdResult(parsedCmd, closeChan)
			}
		}
	}

	// Marshal the response.
	msg, err := createMarshalledReply(responseID, result, jsonErr)
	if err != nil {
		fmt.Printf("Failed to marshal reply: %v\n", err)
		return
	}

	// Write the response.
	err = s.writeHTTPResponseHeaders(r, w.Header(), buf)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	if _, err := buf.Write(msg); err != nil {
		fmt.Printf("Failed to write marshalled reply: %v\n", err)
	}

	// Terminate with newline to maintain compatibility with Bitcoin Core.
	if err := buf.WriteByte('\n'); err != nil {
		fmt.Printf("Failed to append terminating newline to reply: %v\n", err)
	}
}

func (s *rpcServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	rpcServeMux := http.NewServeMux()
	httpServer := &http.Server{
		Handler: rpcServeMux,
	}

	rpcServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		r.Close = true

		// Read and respond to the request.
		s.jsonRPCRead(w, r)
	})

	listeners, err := setupRPCListeners(s.key, s.cert, s.address)
	if err != nil {
		fmt.Printf("Unexpected setupRPCListeners error: %v\n", err)
		return
	}

	s.listeners = listeners

	for _, listener := range listeners {
		s.wg.Add(1)
		go func(listener net.Listener) {
			defer s.wg.Done()
			fmt.Printf("RPC server listening on %s\n", listener.Addr())
			if err := httpServer.Serve(listener); err != nil {
				fmt.Println("httpServer.Serve failed:", err)
				return
			}
			fmt.Printf("RPC listener done for %s\n", listener.Addr())
		}(listener)
	}
}

func (s *rpcServer) Stop() error {
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		fmt.Printf("RPC server is already in the process of shutting down\n")
		return nil
	}

	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			fmt.Printf("Problem shutting down rpc: %v\n", err)
			return err
		}
	}
	s.wg.Wait()
	fmt.Printf("RPC server shutdown complete\n")

	return nil
}

// RequestedProcessShutdown returns a channel that is sent to when an authorized
// RPC client requests the process to shutdown.  If the request can not be read
// immediately, it is dropped.
func (s *rpcServer) RequestedProcessShutdown() <-chan struct{} {
	return s.requestProcessShutdown
}

// shutdownRequestChannel is used to initiate shutdown from one of the
// subsystems using the same code paths as when an interrupt signal is received.
var shutdownRequestChannel = make(chan struct{})

// interruptSignals defines the default signals to catch in order to do a proper
// shutdown.  This may be modified during init depending on the platform.
var interruptSignals = []os.Signal{os.Interrupt}

// interruptListener listens for OS Signals such as SIGINT (Ctrl+C) and shutdown
// requests from shutdownRequestChannel.  It returns a channel that is closed
// when either signal is received.
func interruptListener() <-chan struct{} {
	c := make(chan struct{})
	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, interruptSignals...)

		// Listen for initial shutdown signal and close the returned
		// channel to notify the caller.
		select {
		case sig := <-interruptChannel:
			fmt.Printf("Received signal (%s).  Shutting down...\n",
				sig)

		case <-shutdownRequestChannel:
			fmt.Printf("Shutdown requested.  Shutting down...\n")
		}
		close(c)

		// Listen for repeated signals and display a message so the user
		// knows the shutdown is in progress and the process is not
		// hung.
		for {
			select {
			case sig := <-interruptChannel:
				fmt.Printf("Received signal (%s).  Already "+
					"shutting down...", sig)

			case <-shutdownRequestChannel:
				fmt.Printf("Shutdown requested.  Already " +
					"shutting down...")
			}
		}
	}()

	return c
}

// onionAddr implements the net.Addr interface with two struct fields
type simpleAddr struct {
	net, addr string
}

// String returns the address.
//
// This is part of the net.Addr interface.
func (a simpleAddr) String() string {
	return a.addr
}

// Network returns the network.
//
// This is part of the net.Addr interface.
func (a simpleAddr) Network() string {
	return a.net
}

// timeZeroVal is simply the zero value for a time.Time and is used to avoid
// creating multiple instances.
var timeZeroVal time.Time

type semaphore chan struct{}

func (s semaphore) acquire() { s <- struct{}{} }
func (s semaphore) release() { <-s }

type httpAPIServer struct {
	address string
	listen  *http.Server
	quit    chan struct{}
}

func newHTTPAPIServer(address string) *httpAPIServer {
	return &httpAPIServer{
		address: address,
		quit:    make(chan struct{}),
	}
}

// JSONResponse marshal data into json and write response
func JSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	d, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}

	_, err = w.Write(d)
	return err
}

// httpHandleNextDeposit accept deposits and create a new block, returns the block height.
// Method: POST
// URI: /api/nextdeposit
// The request body is an array of deposits, for example:
//  [{
//     "Address": "1FeDtFhARLxjKUPPkQqEBL78tisenc9znS",
//     "Value":   10000,
//     "N":       4
//  }]
func httpHandleNextDeposit(w http.ResponseWriter, r *http.Request) {
	log.Println("httpHandleNextDeposit, ")
	w.Header().Set("Connection", "close")
	r.Close = true

	if r.Method != http.MethodPost {
		errCode := http.StatusMethodNotAllowed
		http.Error(w, fmt.Sprintf("Accepts POST requests only"), errCode)
	}

	// Read and respond to the request.
	decoder := json.NewDecoder(r.Body)
	var deposits []Deposit
	err := decoder.Decode(&deposits)
	defer func() {
		if err := r.Body.Close(); err != nil {
			fmt.Println("Failed to close response body:", err)
		}
	}()

	if err != nil {
		errCode := http.StatusBadRequest
		http.Error(w, fmt.Sprintf("%d error reading JSON message: %v", errCode, err), errCode)
		return
	}

	newBlock, err := processDeposits(deposits)
	log.Println("Generate new fake block ", newBlock)
	if err != nil {
		errCode := http.StatusBadRequest
		http.Error(w, fmt.Sprintf("%d error processing data: %v", errCode, err), errCode)
		return
	}

	if err := JSONResponse(w, newBlock); err != nil {
		errCode := http.StatusBadRequest
		http.Error(w, fmt.Sprintf("%d error responding: %v", errCode, err), errCode)
		return
	}
}

func (server *httpAPIServer) start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/nextdeposit", httpHandleNextDeposit)

	server.listen = &http.Server{
		Addr:         server.address,
		Handler:      mux,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}

	if err := server.listen.ListenAndServe(); err != nil {
		select {
		case <-server.quit:
			return nil
		default:
			return err
		}
	}
	return nil
}

func (server *httpAPIServer) stop() {
	close(server.quit)
	if server.listen != nil {
		if err := server.listen.Close(); err != nil {
			fmt.Println("http api server shutdown failed")
		}
	}
}

func run() error {
	//flags
	keyFile := flag.String("key", "rpc.key", "btcd rpc key")
	certFile := flag.String("cert", "rpc.cert", "btcd rpc cert")
	address := flag.String("address", "127.0.0.1:8334", "btcd listening address")
	httpAPIAddress := flag.String("api", "127.0.0.1:4122", "http api listening address")

	flag.Parse()

	// Get a channel that will be closed when a shutdown signal has been
	// triggered either from an OS signal such as SIGINT (Ctrl+C) or from
	// another subsystem such as the RPC server.
	interruptedChan := interruptListener()

	server := rpcServer{
		requestProcessShutdown: make(chan struct{}),
		key:                    *keyFile,
		cert:                   *certFile,
		address:                *address,
		maxConcurrentReqs:      10,
	}

	apiServer := newHTTPAPIServer(*httpAPIAddress)

	defer func() {
		apiServer.stop()
		if err := server.Stop(); err != nil {
			fmt.Println("server.Stop failed:", err)
		}
		fmt.Printf("Shutdown complete\n")
	}()

	// Signal process shutdown when the RPC server requests it.
	go func() {
		<-server.RequestedProcessShutdown()
		shutdownRequestChannel <- struct{}{}
	}()

	server.Start()

	go func() {
		fmt.Printf("HTTP API server listening on http://%s\n", *httpAPIAddress)
		err := apiServer.start()
		if err != nil {
			fmt.Printf("HTTP API server failed to start\n")
		}
	}()

	// Wait until the interrupt signal is received from an OS signal or
	// shutdown is requested through one of the subsystems such as the RPC
	// server.
	<-interruptedChan
	return nil
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func init() {
	defaultBlockStore = &BlockStore{
		BlockHashes: make(map[int64]string),
		HashBlocks:  make(map[string]visor.ReadableBlocks),
	}

	if err := json.Unmarshal([]byte(blockString), &initialBlock); err != nil {
		panic(err)
	}
}

func getBlock() (*coin.Block) {
	prev := coin.Block{Head: coin.BlockHeader{Version: 0x02, Time: 100, BkSeq: 98}}
	b := make([]byte, 128)
	rand.Read(b)
	uxHash := cipher.SumSHA256(b)
	txns := coin.Transactions{coin.Transaction{}}

	// valid block is fine
	fee := uint64(121)
	currentTime := uint64(133)
	block, err := coin.NewBlock(prev, currentTime, uxHash, txns, _makeFeeCalc(fee))
	if err != nil {
		panic(err)
	}
	return block
}

var blockString = `{
    "blocks": [
        {
            "header": {
                "version": 0,
                "timestamp": 1477295242,
                "seq": 1,
                "fee": 20732,
                "prev_hash": "f680fe1f068a1cd5c3ef9194f91a9bc3cacffbcae4a32359a3c014da4ef7516f",
                "hash": "662835cc081e037561e1fe05860fdc4b426f6be562565bfaa8ec91be5675064a"
            },
            "body": {
                "txns": [
                    {
                        "length": 608,
                        "type": 0,
                        "txid": "662835cc081e037561e1fe05860fdc4b426f6be562565bfaa8ec91be5675064a",
                        "inner_hash": "37f1111bd83d9c995b9e48511bd52de3b0e440dccbf6d2cfd41dee31a10f1aa4",
                        "sigs": [
                            "ef0b8e1465557e6f21cb2bfad17136188f0b9bd54bba3db76c3488eb8bc900bc7662e3fe162dd6c236d9e52a7051a2133855081a91f6c1a63e1fce2ae9e3820e00",
                            "800323c8c22a2c078cecdfad35210902f91af6f97f0c63fe324e0a9c2159e9356f2fbbfff589edea5a5c24453ef5fc0cd5929f24bebee28e37057acd6d42f3d700",
                            "ca6a6ef5f5fb67490d88ddeeee5e5d11055246613b03e7ed2ad5cc82d01077d262e2da56560083928f5389580ae29500644719cf0e82a5bf065cecbed857598400",
                            "78ddc117607159c7b4c76fc91deace72425f21f2df5918d44d19a377da68cc610668c335c84e2bb7a8f16cd4f9431e900585fc0a3f1024b722b974fcef59dfd500",
                            "4c484d44072e23e97a437deb03a85e3f6eca0bd8875031efe833e3c700fc17f91491969b9864b56c280ef8a68d18dd728b211ce1d46fe477fe3104d73d55ad6501"
                        ],
                        "inputs": [
                            "4bd7c68ecf3039c2b2d8c26a5e2983e20cf53b6d62b099e7786546b3c3f600f9",
                            "f9e39908677cae43832e1ead2514e01eaae48c9a3614a97970f381187ee6c4b1",
                            "7e8ac23a2422b4666ff45192fe36b1bd05f1285cf74e077ac92cabf5a7c1100e",
                            "b3606a4f115d4161e1c8206f4fb5ac0e91551c40d0ee6fe40c86040d2faacac0",
                            "305f1983f5b630bba27e2777c229c725b6b57f37a6ddee138d1d82ae56311909"
                        ],
                        "outputs": [
                            {
                                "uxid": "574d7e5afaefe4ee7e0adf6ce1971d979f038adc8ebbd35771b2c19b0bad7e3d",
                                "dst": "cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW",
                                "coins": "1",
                                "hours": 3455
                            },
                            {
                                "uxid": "6d8a9c89177ce5e9d3b4b59fff67c00f0471fdebdfbb368377841b03fc7d688b",
                                "dst": "fyqX5YuwXMUs4GEUE3LjLyhrqvNztFHQ4B",
                                "coins": "5",
                                "hours": 3455
                            }
                        ]
                    }
                ]
            }
        }
    ]
}`

func _makeFeeCalc(fee uint64) coin.FeeCalculator {
	return func(t *coin.Transaction) (uint64, error) {
		return fee, nil
	}
}
