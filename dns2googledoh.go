// Program dns2googledoh turns DNS queries into Domain-Fronted DNS-over-HTTP
// queries to Google.
package main

/*
 * dns2googledoh.go
 * Proxies DNS over domain-fronted Google DoH
 * By J. Stuart McMurray
 * Created 20190921
 * Last Modified 20190921
 */

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
)

const (
	/* buflen is the size for the UDP query buffer */
	buflen = 2048
	/* host is the host header to use to get to Google's DoH */
	host = "dns.google.com"
)

func main() {
	var (
		sni = flag.String(
			"sni",
			"youtube.com",
			"TLS `SNI`",
		)
		laddr = flag.String(
			"listen",
			"0.0.0.0:5353",
			"Listen `address`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %v [options]

Proxies DNS queries to a DoH server, possibly using domain fronting.

Options:
`,
			os.Args[0],
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	/* Make sure we have a server to call to */
	if "" == *sni {
		log.Fatalf("SNI not set (-sni)")
	}

	/* Listen for DNS queries */
	la, err := net.ResolveUDPAddr("udp", *laddr)
	if nil != err {
		log.Fatalf("Unable to resolve UDP address %v: %v", *laddr, err)
	}
	uc, err := net.ListenUDP("udp", la)
	if nil != err {
		log.Fatalf("Unable to listen on %v: %v", la, err)
	}
	log.Printf("Listening for DNS queries on %v", uc.LocalAddr())

	/* Handle DNS queries */
	pool := &sync.Pool{New: func() interface{} {
		return make([]byte, buflen)
	}}
	for {
		/* Get a query */
		b := pool.Get().([]byte)
		n, a, err := uc.ReadFrom(b)
		if nil != err {
			log.Fatalf("Error getting UDP query: %v", err)
		}
		/* Proxy and return it */
		go func() {
			handleQuery(uc, a, b[:n], *sni)
			pool.Put(b)
		}()
	}
}

/* handleQuery proxies the query to a DoH server and returns the result */
func handleQuery(uc *net.UDPConn, a net.Addr, b []byte, sni string) {
	tag := a.String()

	/* Make sure the query is a DNS query */
	var m dnsmessage.Message
	if err := m.Unpack(b); nil != err {
		log.Printf("[%v] Invalid query: %v", tag, err)
		return
	}

	/* We only support one question at a time */
	switch len(m.Questions) {
	case 0:
		log.Printf("[%v] No questions in query", tag)
		return
	case 1: /* This is what we expect */
	default:
		log.Printf(
			"[%v] Got %v questions in query, but only "+
				"1 question is supported",
			tag,
			len(m.Questions),
		)
		return
	}

	/* We'll need to stick the ID in the repsonse */
	id := m.ID

	/* Now that we've parsed the query, make a better logging tag */
	tag = fmt.Sprintf(
		"%s-%s/%s",
		a,
		m.Questions[0].Name,
		m.Questions[0].Type,
	)

	/* Roll an HTTP request for the DNS query */
	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf(
			"https://"+sni+"/resolve"+
				"?name=%s"+
				"&type=%d"+
				"&ct=application/dns-message",
			m.Questions[0].Name,
			m.Questions[0].Type,
		),
		nil,
	)
	if nil != err {
		log.Printf("[%v] Error creating HTTPS request: %v", tag, err)
		return
	}
	req.Host = host
	o, err := httputil.DumpRequest(req, true) /* DEBUG */
	if nil != err {
		panic(err)
	} /* DEBUG */
	log.Printf("o: %q", o) /* DEBUG */

	/* Send forth the request */
	res, err := http.DefaultClient.Do(req)
	if nil != err {
		log.Printf("[%v] Error making HTTPS query: %v", tag, err)
		return
	}
	defer res.Body.Close()

	/* Make sure we got it back */
	rb, err := ioutil.ReadAll(res.Body)
	if nil != err {
		log.Printf("[%v] Error reading HTTPS response: %v", tag, err)
		return
	}
	if http.StatusOK != res.StatusCode {
		if 0 == len(rb) {
			log.Printf(
				"[%v] Non-OK HTTP response: %v",
				tag,
				res.Status,
			)
		} else {
			log.Printf("[%v] Non-OK HTTP response: %v (%q)",
				tag,
				res.Status,
				rb,
			)
		}
		return
	}
	if 0 == len(rb) {
		log.Printf("[%v] Empty HTTPS response body", a)
		return
	}

	/* Make sure the body is also DNS and put back the ID */
	if err := m.Unpack(rb); nil != err {
		log.Printf("[%v] Invalid DNS response %q: %v", a, rb, err)
		return
	}
	m.ID = id

	/* Send the response back */
	rb, err = m.AppendPack(rb[:0])
	if nil != err {
		log.Printf("[%v] Error packing DNS response: %v", a, err)
		return
	}
	if _, err := uc.WriteTo(rb, a); nil != err {
		log.Printf("[%v] Error sending response: %v", err)
	}
	log.Printf("[%v] %s %s", a, m.Questions[0].Name, m.Questions[0].Type)
}
