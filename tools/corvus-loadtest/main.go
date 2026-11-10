// Command corvus-loadtest exercises the Corvus waiting room PoC over HTTP.
//
//	corvus-loadtest scenario -base http://host:8081 -key <debug-key>
//	corvus-loadtest load     -base http://host:8081 -key <debug-key> -users 1200
//	corvus-loadtest bench    -base http://host:8081 -key <debug-key> -workers 50 -duration 15s
//
// Run it from a machine other than the gateway so the load generator does not
// skew the gateway's measurements.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: corvus-loadtest <scenario|load|bench> [flags]")
		os.Exit(2)
	}
	var ok bool
	switch os.Args[1] {
	case "scenario":
		ok = runScenario(os.Args[2:])
	case "load":
		ok = runLoad(os.Args[2:])
	case "bench":
		ok = runBench(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		os.Exit(2)
	}
	if !ok {
		os.Exit(1)
	}
}

// ------------------------------------------------------------------ HTTP client

func newTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        4096,
		MaxIdleConnsPerHost: 4096,
		IdleConnTimeout:     60 * time.Second,
	}
}

// user is one browser: its own cookie jar, shared connection pool.
type user struct {
	hc   *http.Client
	base string
}

func newUser(base string, tr *http.Transport) *user {
	jar, _ := cookiejar.New(nil)
	return &user{
		base: base,
		hc: &http.Client{
			Jar: jar, Transport: tr, Timeout: 15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

type wrStatus struct {
	Status   string  `json:"status"`
	Rank     int     `json:"rank"`
	Ticket   int     `json:"ticket"`
	Waiting  int     `json:"waiting"`
	Active   int     `json:"active"`
	Capacity int     `json:"capacity"`
	Poll     float64 `json:"poll"`
}

func (u *user) do(method, path string, hdr map[string]string) (int, []byte, error) {
	req, err := http.NewRequest(method, u.base+path, nil)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := u.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

// page requests a protected page like a browser would. The gateway's cookie
// check (a 302 back to the same URL with a queue cookie) is followed once, like
// a browser; any other redirect (e.g. to the waiting page) is returned as-is.
func (u *user) page(path string) (int, error) {
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest("GET", u.base+path, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Accept", "text/html")
		resp, err := u.hc.Do(req)
		if err != nil {
			return 0, err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 302 && resp.Header.Get("Location") == path && i == 0 {
			continue
		}
		return resp.StatusCode, nil
	}
	return 0, fmt.Errorf("redirect loop on %s (cookies rejected?)", path)
}

func (u *user) status(room string) (wrStatus, error) {
	var s wrStatus
	var code int
	var body []byte
	var err error
	// Like the waiting page: back off and retry when the per-session budget (429) is hit.
	for attempt := 0; attempt < 4; attempt++ {
		code, body, err = u.do("GET", "/__corvus/wr/"+room+"/status", nil)
		if err != nil || code != http.StatusTooManyRequests {
			break
		}
		time.Sleep(time.Duration(400*(attempt+1)) * time.Millisecond)
	}
	if err != nil {
		return s, err
	}
	if code != 200 {
		return s, fmt.Errorf("status http %d: %s", code, strings.TrimSpace(string(body)))
	}
	return s, json.Unmarshal(body, &s)
}

func (u *user) leave(room string) error {
	// like a browser on the waiting page: /leave accepts same-origin requests only
	hdr := map[string]string{"Origin": strings.TrimRight(u.base, "/")}
	code, _, err := u.do("POST", "/__corvus/wr/"+room+"/leave", hdr)
	for attempt := 0; err == nil && code == http.StatusTooManyRequests && attempt < 3; attempt++ {
		time.Sleep(time.Duration(400*(attempt+1)) * time.Millisecond)
		code, _, err = u.do("POST", "/__corvus/wr/"+room+"/leave", hdr)
	}
	if err == nil && code != 200 {
		err = fmt.Errorf("leave http %d", code)
	}
	return err
}

type roomStats struct {
	Active   int `json:"active"`
	Waiting  int `json:"waiting"`
	Capacity int `json:"capacity"`
}

func stats(u *user, room, key string) (roomStats, error) {
	var s roomStats
	code, body, err := u.do("GET", "/__corvus/wr/"+room+"/stats", map[string]string{"X-Corvus-Debug-Key": key})
	if err != nil {
		return s, err
	}
	if code != 200 {
		return s, fmt.Errorf("stats http %d (is -key correct?)", code)
	}
	return s, json.Unmarshal(body, &s)
}

func reset(u *user, room, key string) error {
	code, _, err := u.do("POST", "/__corvus/wr/"+room+"/reset", map[string]string{"X-Corvus-Debug-Key": key})
	if err == nil && code != 200 {
		err = fmt.Errorf("reset http %d (is -key correct?)", code)
	}
	return err
}

// ------------------------------------------------------------------ scenario

type checker struct{ pass, fail int }

func (c *checker) expect(ok bool, format string, a ...any) {
	if ok {
		c.pass++
		fmt.Printf("  PASS  %s\n", fmt.Sprintf(format, a...))
	} else {
		c.fail++
		fmt.Printf("  FAIL  %s\n", fmt.Sprintf(format, a...))
	}
}

// runScenario checks the waiting-room semantics deterministically against the
// "fifo" room (capacity 3, active_ttl 15s, grant_ttl 5s, queue_ttl 10s, refresh 5s).
func runScenario(args []string) bool {
	fs := flag.NewFlagSet("scenario", flag.ExitOnError)
	base := fs.String("base", "http://127.0.0.1:8081", "gateway base URL")
	key := fs.String("key", "", "debug key (X-Corvus-Debug-Key)")
	fs.Parse(args)

	const room, path = "fifo", "/fifo/page"
	tr := newTransport()
	admin := newUser(*base, tr)
	mk := func() *user { return newUser(*base, tr) }
	c := &checker{}
	must := func(err error) {
		if err != nil {
			fmt.Println("  ERROR", err)
			c.fail++
		}
	}
	st := func(u *user) wrStatus { s, err := u.status(room); must(err); return s }
	pg := func(u *user) int { code, err := u.page(path); must(err); return code }
	act := func() int { s, err := stats(admin, room, *key); must(err); return s.Active }

	fmt.Println("[1] capacity, FIFO order, refresh, multi-tab, leave")
	must(reset(admin, room, *key))
	a, b, cc := mk(), mk(), mk()
	c.expect(pg(a) == 200 && pg(b) == 200 && pg(cc) == 200, "first 3 users admitted directly (capacity 3)")
	d, e, f := mk(), mk(), mk()
	c.expect(pg(d) == 302 && pg(e) == 302 && pg(f) == 302, "users 4-6 redirected to the waiting page")
	sd, se, sf := st(d), st(e), st(f)
	c.expect(sd.Rank == 1 && se.Rank == 2 && sf.Rank == 3, "ranks are 1,2,3 (got %d,%d,%d)", sd.Rank, se.Rank, sf.Rank)
	c.expect(sd.Ticket < se.Ticket && se.Ticket < sf.Ticket, "tickets increase in arrival order (%d<%d<%d)", sd.Ticket, se.Ticket, sf.Ticket)
	c.expect(pg(d) == 302, "refresh while waiting stays in the queue")
	sd2 := st(d)
	c.expect(sd2.Rank == 1 && sd2.Ticket == sd.Ticket, "refresh keeps the same ticket and rank (ticket %d→%d)", sd.Ticket, sd2.Ticket)
	c.expect(pg(a) == 200 && act() == 3, "second tab of an admitted user does not take a new slot (active=3)")
	must(a.leave(room))
	sd = st(d)
	c.expect(sd.Status == "active", "after one leave, the head of the line (D) is admitted")
	se, sf = st(e), st(f)
	c.expect(se.Rank == 1 && sf.Rank == 2, "remaining waiters move up (E=%d, F=%d)", se.Rank, sf.Rank)
	c.expect(pg(d) == 200, "admitted waiter can load the protected page")
	must(f.leave(room))
	se = st(e)
	c.expect(se.Rank == 1 && se.Waiting == 1, "leaving the queue removes the waiter (waiting=%d)", se.Waiting)
	must(b.leave(room))
	c.expect(st(e).Status == "active", "next leave admits E")
	c.expect(act() == 3, "active never exceeds capacity (active=%d)", act())

	fmt.Println("[2] granted slot expires if the waiter never comes back (grant_ttl 5s)")
	must(reset(admin, room, *key))
	g, h, i := mk(), mk(), mk()
	pg(g)
	pg(h)
	pg(i)
	l := mk()
	pg(l)
	st(l)
	must(h.leave(room)) // promotes L, but L never polls again
	c.expect(act() == 3, "L granted a slot on release (active=3)")
	time.Sleep(6500 * time.Millisecond)
	c.expect(act() == 2, "unclaimed grant released after grant_ttl (active=%d)", act())

	fmt.Println("[3] abandoned waiter is skipped (queue_ttl 10s)")
	must(reset(admin, room, *key))
	g, h, i = mk(), mk(), mk()
	pg(g)
	pg(h)
	pg(i)
	j, k := mk(), mk()
	pg(j)
	sj := st(j) // J polls once, then disappears
	pg(k)
	for t := 0; t < 6; t++ { // K keeps polling for ~11s
		time.Sleep(1850 * time.Millisecond)
		st(k)
	}
	must(g.leave(room))
	c.expect(st(k).Status == "active", "K admitted ahead of abandoned J")
	sj2 := st(j)
	c.expect(sj2.Status == "waiting" && sj2.Ticket > sj.Ticket, "J returning after abandonment rejoins at the back (ticket %d→%d)", sj.Ticket, sj2.Ticket)

	fmt.Println("[4] inactive sessions expire, heartbeats keep sessions alive (active_ttl 15s)")
	must(reset(admin, room, *key))
	x, y := mk(), mk()
	pg(x)
	pg(y)
	for t := 0; t < 3; t++ { // Y browses every 6s (> refresh 5s → heartbeat)
		time.Sleep(6 * time.Second)
		c.expect(pg(y) == 200, "Y still admitted at t=%ds", (t+1)*6)
	}
	c.expect(act() == 1, "X expired after 15s idle, Y kept alive (active=%d)", act())

	must(reset(admin, room, *key))
	fmt.Printf("\nscenario: %d passed, %d failed\n", c.pass, c.fail)
	return c.fail == 0
}

// ------------------------------------------------------------------ load

func runLoad(args []string) bool {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	base := fs.String("base", "http://127.0.0.1:8081", "gateway base URL")
	key := fs.String("key", "", "debug key (X-Corvus-Debug-Key)")
	room := fs.String("room", "event", "waiting room name")
	path := fs.String("path", "/event/", "protected path")
	users := fs.Int("users", 1200, "number of simulated users")
	ramp := fs.Duration("ramp", 5*time.Second, "time over which users arrive")
	hold := fs.Duration("hold", 20*time.Second, "mean time an admitted user stays (±50%)")
	abandon := fs.Float64("abandon", 0.05, "fraction of queued users who give up after the first poll")
	pollScale := fs.Float64("poll-scale", 1.0, "multiplier on the server-suggested poll interval")
	seed := fs.Int64("seed", 1, "random seed")
	fs.Parse(args)

	tr := newTransport()
	admin := newUser(*base, tr)
	if err := reset(admin, *room, *key); err != nil {
		fmt.Println("reset failed:", err)
		return false
	}
	s0, err := stats(admin, *room, *key)
	if err != nil {
		fmt.Println("stats failed:", err)
		return false
	}
	capacity := s0.Capacity
	fmt.Printf("load: room=%s capacity=%d users=%d ramp=%s hold=%s abandon=%.0f%%\n",
		*room, capacity, *users, *ramp, *hold, *abandon*100)

	var (
		clientActive, maxClient, maxServer, maxWaiting         int64
		admitted, direct, queued, abandoned, errs, statusCalls int64
		mu                                                     sync.Mutex
		waits                                                  []time.Duration
		firstErr                                               atomic.Value
	)
	setMax := func(p *int64, v int64) {
		for {
			cur := atomic.LoadInt64(p)
			if v <= cur || atomic.CompareAndSwapInt64(p, cur, v) {
				return
			}
		}
	}
	fail := func(err error) {
		atomic.AddInt64(&errs, 1)
		firstErr.CompareAndSwap(nil, err.Error())
	}

	// Server-side sampler: the authoritative over-capacity check.
	stop := make(chan struct{})
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		last := time.Now()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s, err := stats(admin, *room, *key)
				if err != nil {
					continue
				}
				setMax(&maxServer, int64(s.Active))
				setMax(&maxWaiting, int64(s.Waiting))
				if time.Since(last) >= 5*time.Second {
					last = time.Now()
					fmt.Printf("  t+%3.0fs active=%4d waiting=%5d admitted=%5d abandoned=%4d errors=%d\n",
						time.Since(startTime).Seconds(), s.Active, s.Waiting,
						atomic.LoadInt64(&admitted), atomic.LoadInt64(&abandoned), atomic.LoadInt64(&errs))
				}
			}
		}
	}()

	rng := rand.New(rand.NewSource(*seed))
	type plan struct {
		delay, hold time.Duration
		abandon     bool
	}
	plans := make([]plan, *users)
	for i := range plans {
		plans[i] = plan{
			delay:   time.Duration(float64(*ramp) * float64(i) / float64(*users)),
			hold:    time.Duration(float64(*hold) * (0.5 + rng.Float64())),
			abandon: rng.Float64() < *abandon,
		}
	}

	startTime = time.Now()
	var wg sync.WaitGroup
	for i := 0; i < *users; i++ {
		wg.Add(1)
		go func(p plan) {
			defer wg.Done()
			jitter := rand.New(rand.NewSource(rand.Int63()))
			time.Sleep(p.delay)
			u := newUser(*base, tr)
			t0 := time.Now()
			code, err := u.page(*path)
			if err != nil {
				fail(err)
				return
			}
			switch code {
			case 200:
				atomic.AddInt64(&direct, 1)
			case 302:
				atomic.AddInt64(&queued, 1)
				retries := 0
				for {
					s, err := u.status(*room)
					atomic.AddInt64(&statusCalls, 1)
					if err != nil {
						if retries++; retries > 10 {
							fail(err)
							return
						}
						time.Sleep(time.Second)
						continue
					}
					if s.Status == "active" {
						break
					}
					if s.Status != "waiting" {
						fail(fmt.Errorf("unexpected status %q", s.Status))
						return
					}
					if p.abandon {
						atomic.AddInt64(&abandoned, 1)
						return
					}
					d := time.Duration(s.Poll * *pollScale * (0.8 + 0.4*jitter.Float64()) * float64(time.Second))
					time.Sleep(d)
				}
				if code, err = u.page(*path); err != nil || code != 200 {
					fail(fmt.Errorf("admitted user got %d on protected page (err=%v)", code, err))
					return
				}
			default:
				fail(fmt.Errorf("unexpected first response %d", code))
				return
			}
			wait := time.Since(t0)
			mu.Lock()
			waits = append(waits, wait)
			mu.Unlock()
			atomic.AddInt64(&admitted, 1)
			setMax(&maxClient, atomic.AddInt64(&clientActive, 1))
			time.Sleep(p.hold)
			atomic.AddInt64(&clientActive, -1) // decrement before leaving → client view ⊆ server view
			if err := u.leave(*room); err != nil {
				fail(err)
			}
		}(plans[i])
	}
	wg.Wait()
	elapsed := time.Since(startTime)
	close(stop)
	<-samplerDone
	sEnd, _ := stats(admin, *room, *key)

	sort.Slice(waits, func(i, j int) bool { return waits[i] < waits[j] })
	expectAdmitted := int64(*users) - abandoned
	fmt.Printf("\nresult (%.1fs)\n", elapsed.Seconds())
	fmt.Printf("  admitted          %d / %d (direct %d, via queue %d, abandoned %d)\n", admitted, expectAdmitted, direct, queued-abandoned, abandoned)
	fmt.Printf("  max active server %d  (capacity %d)\n", maxServer, capacity)
	fmt.Printf("  max active client %d\n", maxClient)
	fmt.Printf("  max waiting       %d\n", maxWaiting)
	fmt.Printf("  wait time         p50 %s  p95 %s  max %s\n", pct(waits, 50), pct(waits, 95), pct(waits, 100))
	fmt.Printf("  status polls      %d\n", statusCalls)
	fmt.Printf("  errors            %d", errs)
	if e := firstErr.Load(); e != nil {
		fmt.Printf("  (first: %v)", e)
	}
	fmt.Printf("\n  end state         active=%d waiting=%d\n", sEnd.Active, sEnd.Waiting)

	ok := true
	check := func(cond bool, msg string) {
		if cond {
			fmt.Println("  PASS ", msg)
		} else {
			fmt.Println("  FAIL ", msg)
			ok = false
		}
	}
	fmt.Println()
	check(maxServer <= int64(capacity), "server never exceeded capacity")
	check(maxClient <= int64(capacity), "no more than capacity users held admission at once")
	check(admitted == expectAdmitted, "every non-abandoning user was admitted")
	check(errs == 0, "no errors")
	check(sEnd.Active == 0 && sEnd.Waiting == 0, "room drained at the end")
	return ok
}

var startTime time.Time

func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	i := (len(d) - 1) * p / 100
	return d[i].Round(time.Millisecond)
}

// ------------------------------------------------------------------ bench

// runBench compares request latency on an unprotected path with a protected
// path whose admission token is already held (the steady-state fast path).
func runBench(args []string) bool {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	base := fs.String("base", "http://127.0.0.1:8081", "gateway base URL")
	key := fs.String("key", "", "debug key (X-Corvus-Debug-Key)")
	room := fs.String("room", "event", "waiting room name")
	protected := fs.String("path", "/event/bench", "protected path")
	baseline := fs.String("baseline", "/bench", "unprotected path")
	workers := fs.Int("workers", 50, "concurrent workers")
	dur := fs.Duration("duration", 15*time.Second, "duration per measurement")
	fs.Parse(args)

	tr := newTransport()
	admin := newUser(*base, tr)
	if err := reset(admin, *room, *key); err != nil {
		fmt.Println("reset failed:", err)
		return false
	}
	holder := newUser(*base, tr)
	if code, err := holder.page(*protected); err != nil || code != 200 {
		fmt.Println("could not obtain admission:", code, err)
		return false
	}

	measure := func(name, path string) {
		var mu sync.Mutex
		var lat []time.Duration
		var errs int64
		deadline := time.Now().Add(*dur)
		var wg sync.WaitGroup
		for w := 0; w < *workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				local := make([]time.Duration, 0, 4096)
				for time.Now().Before(deadline) {
					t := time.Now()
					code, err := holder.page(path)
					if err != nil || code != 200 {
						atomic.AddInt64(&errs, 1)
						continue
					}
					local = append(local, time.Since(t))
				}
				mu.Lock()
				lat = append(lat, local...)
				mu.Unlock()
			}()
		}
		wg.Wait()
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		fmt.Printf("  %-26s %7.0f req/s   p50 %-8s p95 %-8s p99 %-8s errors %d\n", name,
			float64(len(lat))/dur.Seconds(), pct(lat, 50), pct(lat, 95), pct(lat, 99), errs)
	}
	fmt.Printf("bench: workers=%d duration=%s each\n", *workers, *dur)
	measure("baseline "+*baseline, *baseline)
	measure("protected "+*protected, *protected)
	measure("baseline (repeat)", *baseline)
	_ = holder.leave(*room)
	return true
}
