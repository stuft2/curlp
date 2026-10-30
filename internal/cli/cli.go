// Package cli implements knok's command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stuft2/knok/internal/auth"
	"github.com/stuft2/knok/internal/config"
)

var Version = "v0-dev"

const usage = `Usage: knok [options] URL
       knok cache clear

HTTP requests with exact-host authentication from YAML configuration.

  -X, --request METHOD    Request method (default GET, or POST with data)
  -H, --header NAME:VALUE Add a request header (repeatable)
  -d, --data DATA         Body: literal text, @file, or @- for stdin
  -i, --include           Include final response status and headers
  -o, --output FILE       Write response to FILE (- means stdout)
  -L, --location          Follow up to ten same-origin redirects
      --timeout DURATION Overall timeout (default 30s; seconds also accepted)
      --config FILE      Read this configuration instead of the default
      --no-auth          Disable configured authentication
      --help             Show help
      --version          Show version
`

type options struct {
	url, method, data, output, config                 string
	headers                                           []string
	hasData, include, location, noAuth, help, version bool
	timeout                                           time.Duration
}

func parse(args []string) (options, error) {
	o := options{timeout: 30 * time.Second}
	flags := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" && flags {
			flags = false
			continue
		}
		if !flags || !strings.HasPrefix(arg, "-") || arg == "-" {
			if o.url != "" {
				return o, fmt.Errorf("expected exactly one URL")
			}
			o.url = arg
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		switch name {
		case "--help", "--version", "--no-auth", "-i", "--include", "-L", "--location":
			if hasValue {
				return o, fmt.Errorf("option does not accept a value")
			}
			switch name {
			case "--help":
				o.help = true
			case "--version":
				o.version = true
			case "--no-auth":
				o.noAuth = true
			case "-i", "--include":
				o.include = true
			default:
				o.location = true
			}
		case "-X", "--request", "-H", "--header", "-d", "--data", "-o", "--output", "--config", "--timeout":
			if !hasValue {
				i++
				if i >= len(args) {
					return o, fmt.Errorf("missing option value")
				}
				value = args[i]
			}
			switch name {
			case "-X", "--request":
				if value == "" {
					return o, fmt.Errorf("empty request method")
				}
				o.method = value
			case "-H", "--header":
				o.headers = append(o.headers, value)
			case "-d", "--data":
				if o.hasData {
					return o, fmt.Errorf("data option may only be supplied once")
				}
				o.data = value
				o.hasData = true
			case "-o", "--output":
				if value == "" {
					return o, fmt.Errorf("empty output path")
				}
				o.output = value
			case "--config":
				if value == "" {
					return o, fmt.Errorf("empty config path")
				}
				o.config = value
			case "--timeout":
				d, err := time.ParseDuration(value)
				if err != nil {
					if seconds, e := strconv.ParseFloat(value, 64); e == nil && seconds > 0 && seconds <= float64((1<<63-1)/int64(time.Second)) {
						d = time.Duration(seconds * float64(time.Second))
						err = nil
					}
				}
				if err != nil || d <= 0 {
					return o, fmt.Errorf("timeout must be a positive duration or seconds")
				}
				o.timeout = d
			}
		default:
			return o, fmt.Errorf("unknown option (see --help)")
		}
	}
	if !o.help && !o.version && o.url == "" {
		return o, fmt.Errorf("expected exactly one URL")
	}
	return o, nil
}

// Runner permits callers to supply a configured transport (for example in offline tests).
// Redirect and timeout policy are always controlled by knok.
type Runner struct{ Client *http.Client }

func (r Runner) Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	fail := func(code int, err error) int { fmt.Fprintln(errOut, "knok:", err); return code }
	if len(args) == 2 && args[0] == "cache" && args[1] == "clear" {
		c, err := auth.DefaultCache()
		if err != nil {
			return fail(1, fmt.Errorf("locate token cache"))
		}
		if err := c.Clear(); err != nil {
			return fail(1, err)
		}
		return 0
	}
	o, err := parse(args)
	if err != nil {
		return fail(2, err)
	}
	if o.help {
		fmt.Fprint(out, usage)
		return 0
	}
	if o.version {
		fmt.Fprintln(out, "knok", Version)
		return 0
	}
	u, err := url.Parse(o.url)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return fail(2, fmt.Errorf("URL must be absolute HTTP(S), without userinfo or fragment"))
	}
	if _, err := http.NewRequest("GET", u.String(), nil); err != nil {
		return fail(2, fmt.Errorf("invalid request URL"))
	}
	path := o.config
	if path == "" {
		path, err = config.DefaultPath()
		if err != nil {
			return fail(2, err)
		}
	}
	cfg, err := config.Load(path, o.config != "")
	if err != nil {
		return fail(2, err)
	}
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	var body io.Reader
	if o.hasData {
		switch {
		case o.data == "@-":
			body = in
		case strings.HasPrefix(o.data, "@"):
			f, err := os.Open(strings.TrimPrefix(o.data, "@"))
			if err != nil {
				return fail(1, fmt.Errorf("open request body: %w", err))
			}
			defer f.Close()
			body = f
		default:
			body = strings.NewReader(o.data)
		}
	}
	method := o.method
	if method == "" {
		method = "GET"
		if o.hasData {
			method = "POST"
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return fail(2, fmt.Errorf("invalid request method or URL"))
	}
	req.Header.Set("User-Agent", "knok/"+Version)
	for _, h := range o.headers {
		name, value, ok := strings.Cut(h, ":")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if !ok || !headerName(name) || strings.ContainsAny(value, "\r\n\x00") {
			return fail(2, fmt.Errorf("invalid request header"))
		}
		req.Header.Add(name, value)
	}
	if len(req.Header.Values("Authorization")) > 1 {
		return fail(2, fmt.Errorf("duplicate Authorization headers are not allowed"))
	}
	client := http.Client{}
	if r.Client != nil {
		client = *r.Client
	}
	client.Timeout = o.timeout
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if !o.location {
			return http.ErrUseLastResponse
		}
		if len(via) > 10 {
			return fmt.Errorf("redirect limit exceeded")
		}
		if !sameOrigin(via[0].URL, next.URL) {
			return fmt.Errorf("cross-origin redirect blocked")
		}
		return nil
	}
	if h, ok := cfg.Hosts[config.Hostname(u.Hostname())]; ok && !o.noAuth && len(req.Header.Values("Authorization")) == 0 {
		cache, err := auth.DefaultCache()
		if err != nil {
			cache = auth.Cache{}
			fmt.Fprintln(errOut, "knok: token cache unavailable")
		}
		cache.Warn = func(s string) { fmt.Fprintln(errOut, "knok:", s) }
		if err := (auth.Provider{Client: &client, Cache: cache}).Apply(ctx, req, h.Auth); err != nil {
			return fail(1, err)
		}
	}
	res, err := client.Do(req)
	if err != nil {
		// url.Error includes the full URL; report only a safe operation summary.
		if ctx.Err() != nil {
			return fail(1, fmt.Errorf("request timed out or was canceled"))
		}
		if e, ok := err.(*url.Error); ok && (e.Err.Error() == "cross-origin redirect blocked" || e.Err.Error() == "redirect limit exceeded") {
			return fail(1, e.Err)
		}
		return fail(1, fmt.Errorf("HTTP request failed"))
	}
	defer res.Body.Close()
	dst := out
	var file *os.File
	if o.output != "" && o.output != "-" {
		file, err = os.Create(o.output)
		if err != nil {
			return fail(1, fmt.Errorf("open response output: %w", err))
		}
		defer file.Close()
		dst = file
	}
	if o.include {
		if _, err = fmt.Fprintf(dst, "%s %s\r\n", res.Proto, res.Status); err == nil {
			err = res.Header.Write(dst)
		}
		if err == nil {
			_, err = io.WriteString(dst, "\r\n")
		}
		if err != nil {
			return fail(1, fmt.Errorf("write response headers"))
		}
	}
	if _, err := io.Copy(dst, res.Body); err != nil {
		return fail(1, fmt.Errorf("read or write response body failed"))
	}
	if file != nil {
		if err := file.Close(); err != nil {
			return fail(1, fmt.Errorf("close response output failed"))
		}
	}
	return 0
}
func headerName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}
func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return a.Scheme == b.Scheme && config.Hostname(a.Hostname()) == config.Hostname(b.Hostname()) && port(a) == port(b)
}
