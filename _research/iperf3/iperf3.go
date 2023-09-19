package nspeed

import "math/rand"

// port of https://github.com/esnet/iperf/blob/master/src/iperf_util.c#L114

const COOKIE_SIZE = 37

func IPerf3_make_cookie() string {
	rndchars := []rune("abcdefghijklmnopqrstuvwxyz234567")
	cookie := make([]rune, COOKIE_SIZE)
	for i := range cookie {
		cookie[i] = rndchars[rand.Intn(len(rndchars))]
	}
	return string(cookie)
}

/* states */
const (
	IPerf3_State_TEST_START       = 1
	IPerf3_State_TEST_RUNNING     = 2
	IPerf3_State_RESULT_REQUEST   = 3 /* not used */
	IPerf3_State_TEST_END         = 4
	IPerf3_State_STREAM_BEGIN     = 5 /* not used */
	IPerf3_State_STREAM_RUNNING   = 6 /* not used */
	IPerf3_State_STREAM_END       = 7 /* not used */
	IPerf3_State_ALL_STREAMS_END  = 8 /* not used */
	IPerf3_State_PARAM_EXCHANGE   = 9
	IPerf3_State_CREATE_STREAMS   = 10
	IPerf3_State_SERVER_TERMINATE = 11
	IPerf3_State_CLIENT_TERMINATE = 12
	IPerf3_State_EXCHANGE_RESULTS = 13
	IPerf3_State_DISPLAY_RESULTS  = 14
	IPerf3_State_IPERF_START      = 15
	IPerf3_State_IPERF_DONE       = 16
	IPerf3_State_ACCESS_DENIED    = -1
	IPerf3_State_SERVER_ERROR     = -2
)
