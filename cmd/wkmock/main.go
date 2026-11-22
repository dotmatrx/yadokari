// Command wkmock serves the fake WaniKani API for local development.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dotmatrx/yadokari/internal/wkmock"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8642", "listen address")
	fixtures := flag.String("fixtures", "", "fixture directory (default: built-in sample data)")
	token := flag.String("token", "", "accepted API token (default: any)")
	perPage := flag.Int("per-page", 0, "override page size")
	rateLimit := flag.Int("rate-limit", 60, "requests per minute (0 = unlimited)")
	demo := flag.Bool("demo", false, "serve the larger demo data set (level 8, busy forecast)")
	flag.Parse()

	data := wkmock.SampleData(time.Now())
	if *demo {
		data = wkmock.DemoData(time.Now())
	}
	if *fixtures != "" {
		var err error
		if data, err = wkmock.LoadDir(*fixtures); err != nil {
			log.Fatal(err)
		}
	}
	s := wkmock.New(data)
	s.Token, s.PerPage, s.RateLimit = *token, *perPage, *rateLimit

	h := s.Handler()
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(os.Stderr, "%s %s %s\n", time.Now().Format(time.TimeOnly), r.Method, r.URL.RequestURI())
		h.ServeHTTP(w, r)
	})
	fmt.Fprintf(os.Stderr, "wkmock listening on http://%s/v2/ (%d subjects, %d assignments)\n",
		*addr, len(data.Subjects), len(data.Assignments))
	log.Fatal(http.ListenAndServe(*addr, logged))
}
