package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

var testLogLevel = "Error"

// GlobalTestConfig contains the global configuration (after config files have been parsed)
var GlobalTestConfig Config

func init() {
	setDefaults(&GlobalTestConfig)
	InitLogging(&Config{LogLevel: testLogLevel, LogFile: "stderr"})

	TestPeerWaitGroup = &sync.WaitGroup{}

	once.Do(PrintVersion)
}

func assertEq(exp, got interface{}) error {
	if !reflect.DeepEqual(exp, got) {
		return fmt.Errorf("\nWanted \n%#v\nGot\n%#v", exp, got)
	}
	return nil
}

func assertLike(exp string, got string) error {
	regex, err := regexp.Compile(exp)
	if err != nil {
		panic(err.Error())
	}
	if !regex.MatchString(got) {
		return fmt.Errorf("\nWanted \n%#v\nGot\n%#v", exp, got)
	}
	return nil
}

func StartMockLivestatusSource(nr int, numHosts int, numServices int) (listen string) {
	startedChannel := make(chan bool)
	listen = fmt.Sprintf("mock%d_%d.sock", nr, time.Now().Nanosecond())
	TestPeerWaitGroup.Add(1)

	dataFolder := "../t/data"
	tempFolder := ""
	if numHosts > 0 || numServices > 0 {
		// prepare data files
		tempFolder = prepareTmpData(dataFolder, nr, numHosts, numServices)
		dataFolder = tempFolder
	}

	go func() {
		os.Remove(listen)
		l, err := net.Listen("unix", listen)
		if err != nil {
			panic(err.Error())
		}
		defer func() {
			os.Remove(listen)
			l.Close()
			if tempFolder != "" {
				os.RemoveAll(tempFolder)
			}
			TestPeerWaitGroup.Done()
		}()
		startedChannel <- true
		for {
			conn, err := l.Accept()
			if err != nil {
				panic(err.Error())
			}

			req, err := ParseRequest(conn)
			if err != nil {
				panic(err.Error())
			}

			if req.Command != "" {
				conn.Close()
				if req.Command == "COMMAND [0] MOCK_EXIT" {
					return
				}
				continue
			}

			if len(req.Filter) > 0 || len(req.Stats) > 0 {
				conn.Write([]byte("200            3\n[]\n"))
				conn.Close()
				continue
			}

			dat, err := ioutil.ReadFile(fmt.Sprintf("%s/%s.json", dataFolder, req.Table))
			if err != nil {
				panic("could not read file: " + err.Error())
			}
			conn.Write([]byte(dat))
			conn.Close()
		}
	}()
	<-startedChannel
	return
}

func prepareTmpData(dataFolder string, nr int, numHosts int, numServices int) (tempFolder string) {
	tempFolder, err := ioutil.TempDir("", fmt.Sprintf("mockdata%d_", nr))
	if err != nil {
		panic("failed to create temp data folder: " + err.Error())
	}
	// read existing json files and extend hosts and services
	for name, table := range Objects.Tables {
		if table.Virtual || table.GroupBy || table.PassthroughOnly {
			continue
		}
		file, err := os.Create(fmt.Sprintf("%s/%s.json", tempFolder, name))
		if err != nil {
			panic("failed to create temp file: " + err.Error())
		}
		template, err := os.Open(fmt.Sprintf("%s/%s.json", dataFolder, name))
		if err != nil {
			panic("failed to open temp file: " + err.Error())
		}
		if name == "hosts" || name == "services" {
			err = file.Close()
			prepareTmpDataHostService(dataFolder, tempFolder, table, numHosts, numServices)
		} else {
			io.Copy(file, template)
			err = file.Close()
		}
		if err != nil {
			panic("failed to create temp file: " + err.Error())
		}
	}
	return
}

func prepareTmpDataHostService(dataFolder string, tempFolder string, table *Table, numHosts int, numServices int) {
	name := table.Name
	dat, _ := ioutil.ReadFile(fmt.Sprintf("%s/%s.json", dataFolder, name))
	removeFirstLine := regexp.MustCompile("^200.*")
	dat = removeFirstLine.ReplaceAll(dat, []byte{})
	var raw = [][]interface{}{}
	err := json.Unmarshal(dat, &raw)
	if err != nil {
		panic("failed to decode: " + err.Error())
	}
	num := len(raw)
	last := raw[num-1]
	newData := [][]interface{}{}
	if name == "hosts" {
		nameIndex := table.GetColumn("name").Index
		for x := 1; x <= numHosts; x++ {
			newObj := make([]interface{}, len(last))
			copy(newObj, last)
			newObj[nameIndex] = fmt.Sprintf("%s_%d", "testhost", x)
			newData = append(newData, newObj)
		}
	}
	if name == "services" {
		nameIndex := table.GetColumn("host_name").Index
		descIndex := table.GetColumn("description").Index
		for x := 1; x <= numHosts; x++ {
			for y := 1; y < numServices/numHosts; y++ {
				newObj := make([]interface{}, len(last))
				copy(newObj, last)
				newObj[nameIndex] = fmt.Sprintf("%s_%d", "testhost", x)
				newObj[descIndex] = fmt.Sprintf("%s_%d", "testsvc", y)
				newData = append(newData, newObj)
				if len(newData) == numServices {
					break
				}
			}
			if len(newData) == numServices {
				break
			}
		}
	}

	buf := new(bytes.Buffer)
	buf.Write([]byte("["))
	for i, row := range newData {
		enc, _ := json.Marshal(row)
		buf.Write(enc)
		if i < len(newData)-1 {
			buf.Write([]byte(",\n"))
		}
	}
	buf.Write([]byte("]\n"))
	encoded := []byte(fmt.Sprintf("%d %11d\n", 200, len(buf.Bytes())))
	encoded = append(encoded, buf.Bytes()...)
	ioutil.WriteFile(fmt.Sprintf("%s/%s.json", tempFolder, name), encoded, 0644)
}

var TestPeerWaitGroup *sync.WaitGroup

func StartMockMainLoop(sockets []string, extraConfig string) {
	var testConfig = `
Loglevel = "` + testLogLevel + `"

`
	testConfig += extraConfig
	if !strings.Contains(testConfig, "Listen ") {
		testConfig += `Listen = ["test.sock"]
		`
	}

	for i, socket := range sockets {
		testConfig += fmt.Sprintf("[[Connections]]\nname = \"MockCon-%s\"\nid   = \"mockid%d\"\nsource = [\"%s\"]\n\n", socket, i, socket)
	}

	err := ioutil.WriteFile("test.ini", []byte(testConfig), 0644)
	if err != nil {
		panic(err.Error())
	}

	toml.DecodeFile("test.ini", &GlobalTestConfig)
	mainSignalChannel = make(chan os.Signal)
	startedChannel := make(chan bool)

	go func() {
		flagConfigFile = configFiles{"test.ini"}
		TestPeerWaitGroup.Add(1)
		startedChannel <- true
		mainLoop(mainSignalChannel)
		TestPeerWaitGroup.Done()
		os.Remove("test.ini")
	}()
	<-startedChannel
}

// StartTestPeer just call StartTestPeerExtra
func StartTestPeer(numPeers int, numHosts int, numServices int) *Peer {
	return (StartTestPeerExtra(numPeers, numHosts, numServices, ""))
}

// StartTestPeerExtra starts:
//  - a mock livestatus server which responds from status json
//  - a main loop which has the mock server(s) as backend
// It returns a peer with the "mainloop" connection configured
func StartTestPeerExtra(numPeers int, numHosts int, numServices int, extraConfig string) (peer *Peer) {
	sockets := []string{}
	for i := 0; i < numPeers; i++ {
		listen := StartMockLivestatusSource(i, numHosts, numServices)
		sockets = append(sockets, listen)
	}
	StartMockMainLoop(sockets, extraConfig)

	testPeerShutdownChannel := make(chan bool)
	peer = NewPeer(&GlobalTestConfig, &Connection{Source: []string{"doesnotexist", "test.sock"}, Name: "Test", ID: "testid"}, TestPeerWaitGroup, testPeerShutdownChannel)

	// wait till backend is available
	retries := 0
	for {
		res, err := peer.QueryString("GET backends\nColumns: status last_error\nFilter: status = 0\nResponseHeader: fixed16\n\n")
		if err == nil && len(res) == len(sockets) && len(res[0]) > 0 && res[0][0].(float64) == 0 && res[0][1].(string) == "" {
			break
		}
		// recheck every 100ms
		time.Sleep(100 * time.Millisecond)
		retries++
		if retries > 100 {
			if err != nil {
				panic("backend never came online: " + err.Error())
			} else {
				panic("backend never came online")
			}
		}
	}

	// fill tables
	peer.InitAllTables()

	return
}

func StopTestPeer(peer *Peer) (err error) {
	// stop the mock servers
	peer.QueryString("COMMAND [0] MOCK_EXIT")
	// stop the mainloop
	mainSignalChannel <- syscall.SIGTERM
	// stop the test peer
	peer.Stop()
	// wait till all has stoped
	if waitTimeout(TestPeerWaitGroup, 10*time.Second) {
		err = fmt.Errorf("timeout while waiting for peers to stop")
	}
	return
}

func PauseTestPeers(peer *Peer) {
	peer.Stop()
	for _, p := range PeerMap {
		p.Stop()
	}
}

func CheckOpenFilesLimit(b *testing.B, minimum uint64) {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		b.Skip("skipping test, cannot fetch open files limit.")
	}
	if rLimit.Cur < minimum {
		b.Skip(fmt.Sprintf("skipping test, open files limit too low, need at least %d, current: %d", minimum, rLimit.Cur))
	}
}
