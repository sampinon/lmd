package main

import (
	"bufio"
	"bytes"
	"testing"
	"time"
)

func TestRequestHeader(t *testing.T) {
	testRequestStrings := []string{
		"GET hosts\n\n",
		"GET hosts\nColumns: name state\n\n",
		"GET hosts\nColumns: name state\nFilter: state != 1\n\n",
		"GET hosts\nOutputFormat: wrapped_json\n\n",
		"GET hosts\nResponseHeader: fixed16\n\n",
		"GET hosts\nColumns: name state\nFilter: state != 1\nFilter: is_executing = 1\nOr: 2\n\n",
		"GET hosts\nColumns: name state\nFilter: state != 1\nFilter: is_executing = 1\nAnd: 2\nFilter: state = 1\nOr: 2\nFilter: name = test\n\n",
		"GET hosts\nBackends: mockid0\n\n",
		"GET hosts\nLimit: 25\nOffset: 5\n\n",
		"GET hosts\nSort: name asc\nSort: state desc\n\n",
		"GET hosts\nStats: state = 1\nStats: avg latency\nStats: state = 3\nStats: state != 1\nStatsAnd: 2\n\n",
		"GET hosts\nColumns: name\nFilter: name ~~ test\n\n",
		"GET hosts\nColumns: name\nFilter: name !~ Test\n\n",
		"GET hosts\nColumns: name\nFilter: name !~~ test\n\n",
		"GET hosts\nColumns: name\nFilter: custom_variables ~~ TAGS test\n\n",
		"GET hosts\nColumns: name\nFilter: custom_variables = TAGS\n\n",
		"GET hosts\nColumns: name\nFilter: name !=\n\n",
		"COMMAND [123456] TEST\n\n",
		"GET hosts\nColumns: name\nFilter: name = test\nWaitTrigger: all\nWaitObject: test\nWaitTimeout: 10000\nWaitCondition: last_check > 1473760401\n\n",
		"GET hosts\nColumns: name\nFilter: latency != 1.23456789012345\n\n",
		"GET hosts\nColumns: name comments\nFilter: comments >= 1\n\n",
		"GET hosts\nColumns: name contact_groups\nFilter: contact_groups >= test\n\n",
		"GET hosts\nColumns: name\nFilter: last_check >= 123456789\n\n",
		"GET hosts\nColumns: name\nFilter: last_check =\n\n",
	}
	for _, str := range testRequestStrings {
		buf := bufio.NewReader(bytes.NewBufferString(str))
		req, _, err := NewRequest(buf)
		if err != nil {
			t.Fatal(err)
		}
		if err = assertEq(str, req.String()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequestHeaderTable(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\n"))
	req, _, _ := NewRequest(buf)
	if err := assertEq("hosts", req.Table); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderLimit(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\nLimit: 10\n"))
	req, _, _ := NewRequest(buf)
	if err := assertEq(10, req.Limit); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderOffset(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\nOffset: 3\n"))
	req, _, _ := NewRequest(buf)
	if err := assertEq(3, req.Offset); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderColumns(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\nColumns: name state\n"))
	req, _, _ := NewRequest(buf)
	if err := assertEq([]string{"name", "state"}, req.Columns); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderSort(t *testing.T) {
	req, _, _ := NewRequest(bufio.NewReader(bytes.NewBufferString("GET hosts\nColumns: latency state name\nSort: name desc\nSort: state asc\n")))
	req.BuildResponseIndexes(Objects.Tables[req.Table])
	if err := assertEq(SortField{Name: "name", Direction: Desc, Index: 2}, *req.Sort[0]); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(SortField{Name: "state", Direction: Asc, Index: 1}, *req.Sort[1]); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderSortCust(t *testing.T) {
	req, _, _ := NewRequest(bufio.NewReader(bytes.NewBufferString("GET hosts\nColumns: name custom_variables\nSort: custom_variables TEST asc\n")))
	req.BuildResponseIndexes(Objects.Tables[req.Table])
	if err := assertEq(SortField{Name: "custom_variables", Direction: Asc, Index: 1, Args: "TEST"}, *req.Sort[0]); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderFilter1(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\nFilter: name != test\n"))
	req, _, _ := NewRequest(buf)
	if err := assertEq(len(req.Filter), 1); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(req.Filter[0].Column.Name, "name"); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderFilter2(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\nFilter: state != 1\nFilter: name = with spaces \n"))
	req, _, _ := NewRequest(buf)
	if err := assertEq(len(req.Filter), 2); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(req.Filter[0].Column.Name, "state"); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(req.Filter[1].Column.Name, "name"); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(req.Filter[1].StrValue, "with spaces"); err != nil {
		t.Fatal(err)
	}
}

func TestRequestHeaderFilter3(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString("GET hosts\nFilter: state != 1\nFilter: name = with spaces\nOr: 2"))
	req, _, _ := NewRequest(buf)
	if err := assertEq(len(req.Filter), 1); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(len(req.Filter[0].Filter), 2); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(req.Filter[0].GroupOperator, Or); err != nil {
		t.Fatal(err)
	}
}

func TestRequestListFilter(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	res, _ := peer.QueryString("GET hosts\nColumns: name\nFilter: contact_groups >= demo\nSort: name asc")
	if err := assertEq("gearman", res[0][0]); err != nil {
		t.Fatal(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestHeaderMultipleCommands(t *testing.T) {
	buf := bufio.NewReader(bytes.NewBufferString(`COMMAND [1473627610] SCHEDULE_FORCED_SVC_CHECK;demo;Web1;1473627610
Backends: mockid0

COMMAND [1473627610] SCHEDULE_FORCED_SVC_CHECK;demo;Web2;1473627610`))
	req, size, err := NewRequest(buf)
	if err != nil {
		t.Fatal(err)
	}
	if err = assertEq(size, 87); err != nil {
		t.Fatal(err)
	}
	if err = assertEq(req.Command, "COMMAND [1473627610] SCHEDULE_FORCED_SVC_CHECK;demo;Web1;1473627610"); err != nil {
		t.Fatal(err)
	}
	if err = assertEq(req.Backends[0], "mockid0"); err != nil {
		t.Fatal(err)
	}
	req, size, err = NewRequest(buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertEq(size, 67); err != nil {
		t.Fatal(err)
	}
	if err := assertEq(req.Command, "COMMAND [1473627610] SCHEDULE_FORCED_SVC_CHECK;demo;Web2;1473627610"); err != nil {
		t.Fatal(err)
	}
}

type ErrorRequest struct {
	Request string
	Error   string
}

func TestResponseErrorsFunc(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	testRequestStrings := []ErrorRequest{
		{"", "bad request: empty request"},
		{"NOE", "bad request: NOE"},
		{"GET none\nColumns: none", "bad request: table none does not exist"},
		{"GET hosts\nnone", "bad request header: none"},
		{"GET hosts\nNone: blah", "bad request: unrecognized header None: blah"},
		{"GET hosts\nLimit: x", "bad request: limit must be a positive number"},
		{"GET hosts\nLimit: -1", "bad request: limit must be a positive number"},
		{"GET hosts\nOffset: x", "bad request: offset must be a positive number"},
		{"GET hosts\nOffset: -1", "bad request: offset must be a positive number"},
		{"GET hosts\nSort: 1", "bad request: invalid sort header, must be 'Sort: <field> <asc|desc>' or 'Sort: custom_variables <name> <asc|desc>'"},
		{"GET hosts\nSort: name none", "bad request: unrecognized sort direction, must be asc or desc"},
		{"GET hosts\nSort: name", "bad request: invalid sort header, must be 'Sort: <field> <asc|desc>' or 'Sort: custom_variables <name> <asc|desc>'"},
		{"GET hosts\nColumns: name\nSort: state asc", "bad request: sort column state not in result set\nRequest: GET hosts\nColumns: name\nSort: state asc\n\n\nResponse: bad request: sort column state not in result set\n"},
		{"GET hosts\nResponseheader: none", "bad request: unrecognized responseformat, only fixed16 is supported"},
		{"GET hosts\nOutputFormat: csv: none", "bad request: unrecognized outputformat, only json and wrapped_json is supported"},
		{"GET hosts\nStatsAnd: 1", "bad request: not enough filter on stack in StatsAnd: 1"},
		{"GET hosts\nStatsOr: 1", "bad request: not enough filter on stack in StatsOr: 1"},
		{"GET hosts\nWaitTrigger: all", "bad request: WaitTrigger without WaitCondition"},
		{"GET hosts\nWaitTrigger: all\nWaitCondition: last_check > 0", "bad request: WaitTrigger without WaitTimeout"},
		{"GET hosts\nWaitTrigger: all\nWaitCondition: last_check > 0\nWaitTimeout: 10000", "bad request: WaitTrigger without WaitObject"},
		{"GET hosts\nFilter: name", "bad request: filter header, must be Filter: <field> <operator> <value>"},
		{"GET hosts\nFilter: name ~~ *^", "bad request: invalid regular expression: error parsing regexp: missing argument to repetition operator: `*` in filter Filter: name ~~ *^"},
		{"GET hosts\nStats: name", "bad request: stats header, must be Stats: <field> <operator> <value> OR Stats: <sum|avg|min|max> <field>"},
		{"GET hosts\nStats: avg none", "bad request: unrecognized column from stats: none in Stats: avg none"},
		{"GET hosts\nFilter: name !=\nAnd: x", "bad request: and must be a positive number in: And: x"},
		{"GET hosts\nColumns: name\nFilter: custom_variables =", `bad request: custom variable filter must have form "Filter: custom_variables <op> <variable> [<value>]" in Filter: custom_variables =`},
		{"GET hosts\nKeepalive: broke", `bad request: must be 'on' or 'off' in Keepalive: broke`},
	}

	for _, er := range testRequestStrings {
		_, err := peer.QueryString(er.Request)
		if err == nil {
			t.Fatalf("No Error in Request: " + er.Request)
		}
		if err = assertEq(er.Error, err.Error()); err != nil {
			t.Error("Request: " + er.Request)
			t.Fatalf(err.Error())
		}
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestNestedFilter(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	if err := assertEq(1, len(PeerMap)); err != nil {
		t.Error(err)
	}

	query := `GET services
Columns: host_name description state peer_key
Filter: description ~~ session
Filter: display_name ~~ session
Or: 2
Filter: description !~~ database
Filter: display_name !~~ database
And: 2
And: 2
Limit: 100
Offset: 0
Sort: host_name asc
Sort: description asc
OutputFormat: wrapped_json
ResponseHeader: fixed16
`
	res, err := peer.QueryString(query)
	if err = assertEq(3, len(res)); err != nil {
		t.Error(err)
	}

	if err = assertEq("tomcat", res[0][0]); err != nil {
		t.Error(err)
	}
	if err = assertEq("session_active", res[0][1]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestStats(t *testing.T) {
	peer := StartTestPeer(4, 10, 10)
	PauseTestPeers(peer)

	if err := assertEq(4, len(PeerMap)); err != nil {
		t.Error(err)
	}

	res, err := peer.QueryString("GET hosts\nColumns: name latency\n\n")
	if err = assertEq(40, len(res)); err != nil {
		t.Error(err)
	}

	res, err = peer.QueryString("GET hosts\nStats: sum latency\nStats: avg latency\nStats: min has_been_checked\nStats: max execution_time\nStats: name !=\n")

	if err = assertEq(9.6262454988, res[0][0]); err != nil {
		t.Error(err)
	}
	if err = assertEq(0.24065613746999998, res[0][1]); err != nil {
		t.Error(err)
	}
	if err = assertEq(float64(1), res[0][2]); err != nil {
		t.Error(err)
	}
	if err = assertEq(4.010726, res[0][3]); err != nil {
		t.Error(err)
	}
	if err = assertEq(float64(40), res[0][4]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestStatsGroupBy(t *testing.T) {
	peer := StartTestPeer(4, 0, 0)
	PauseTestPeers(peer)

	if err := assertEq(4, len(PeerMap)); err != nil {
		t.Error(err)
	}

	res, err := peer.QueryString("GET hosts\nColumns: name\nStats: avg latency\n\n")
	if err = assertEq(12, len(res)); err != nil {
		t.Error(err)
	}
	if err = assertEq("gearman", res[1][0]); err != nil {
		t.Error(err)
	}
	if err = assertEq(0.051033973694, res[1][1]); err != nil {
		t.Error(err)
	}

	res, err = peer.QueryString("GET hosts\nColumns: name alias\nStats: avg latency\n\n")
	if err = assertEq(12, len(res)); err != nil {
		t.Error(err)
	}
	if err = assertEq("gearman", res[1][0]); err != nil {
		t.Error(err)
	}
	if err = assertEq("gearman", res[1][1]); err != nil {
		t.Error(err)
	}
	if err = assertEq(0.051033973694, res[1][2]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestStatsEmpty(t *testing.T) {
	peer := StartTestPeer(2, 0, 0)
	PauseTestPeers(peer)

	res, err := peer.QueryString("GET hosts\nFilter: check_type = 15\nStats: sum percent_state_change\nStats: min percent_state_change\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = assertEq(1, len(res)); err != nil {
		t.Fatal(err)
	}
	if err = assertEq(float64(0), res[0][0]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestStatsBroken(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	res, err := peer.QueryString("GET hosts\nStats: sum name\nStats: avg contacts\nStats: min plugin_output\n")
	if err = assertEq(float64(0), res[0][0]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestRefs(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	res1, err := peer.QueryString("GET hosts\nColumns: name latency check_command\nLimit: 1\n\n")
	if err = assertEq(1, len(res1)); err != nil {
		t.Error(err)
	}

	res2, err := peer.QueryString("GET services\nColumns: host_name host_latency host_check_command\nFilter: host_name = " + res1[0][0].(string) + "\nLimit: 1\n\n")

	if err = assertEq(res1[0], res2[0]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestBrokenColumns(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	res, err := peer.QueryString("GET hosts\nColumns: host_name alias\nFilter: host_name = gearman\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = assertEq(1, len(res)); err != nil {
		t.Fatal(err)
	}
	if err = assertEq("gearman", res[0][0]); err != nil {
		t.Error(err)
	}
	if err = assertEq("gearman", res[0][1]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestGroupByTable(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	res, err := peer.QueryString("GET servicesbyhostgroup\nColumns: host_name description host_groups groups host_alias host_address\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = assertEq(116, len(res)); err != nil {
		t.Fatal(err)
	}
	if err = assertEq("Test Business Process", res[0][0]); err != nil {
		t.Error(err)
	}
	if err = assertEq("Business Process", res[0][5]); err != nil {
		t.Error(err)
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}

func TestRequestBlocking(t *testing.T) {
	peer := StartTestPeer(1, 0, 0)
	PauseTestPeers(peer)

	start := time.Now()

	// start long running query in background
	go func() {
		_, err1 := peer.QueryString("GET hosts\nColumns: name latency check_command\nLimit: 1\nWaitTrigger: all\nWaitObject: test\nWaitTimeout: 5000\nWaitCondition: last_check > 1473760401\n\n")
		if err1 != nil {
			t.Fatal(err1)
		}
	}()

	// test how long next query will take
	_, err2 := peer.QueryString("GET hosts\nColumns: name latency check_command\nLimit: 1\n\n")
	if err2 != nil {
		t.Fatal(err2)
	}

	elapsed := time.Since(start)
	if elapsed.Seconds() > 3 {
		t.Error("query2 should return immediately")
	}

	if err := StopTestPeer(peer); err != nil {
		panic(err.Error())
	}
}
