package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func runRecord(args []string) int {
	flags := flag.NewFlagSet("record", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", "127.0.0.1:8080", "loopback IP:port to listen on")
	target := flags.String("target", "", "upstream HTTP or HTTPS URL (required)")
	output := flags.String("output", "", "recording file path (required)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stdout)
			fmt.Println("Usage: prolane record --target URL --output PATH [--listen IP:port]\n\nRecording is not implemented yet; this command only validates options.")
			flags.PrintDefaults()
			return 0
		}
		fmt.Fprintln(os.Stderr, "prolane record:", err)
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "prolane record: positional arguments are not supported; use --help")
		return 2
	}
	if err := validateRecordOptions(*listen, *target, *output); err != nil {
		fmt.Fprintln(os.Stderr, "prolane record:", err)
		return 2
	}

	fmt.Fprintln(os.Stderr, "prolane record: recording is not implemented yet; no traffic was forwarded or output file created")
	return 1
}

func validateRecordOptions(listen, target, output string) error {
	host, port, err := net.SplitHostPort(listen)
	listenIP, ipErr := netip.ParseAddr(host)
	listenPort, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || ipErr != nil || !listenIP.IsLoopback() || portErr != nil || listenPort == 0 {
		return errors.New("--listen must use a loopback IP and a numeric port from 1 to 65535")
	}

	upstream, err := url.Parse(target)
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") ||
		upstream.Hostname() == "" || upstream.User != nil || upstream.RawQuery != "" ||
		upstream.ForceQuery || strings.Contains(target, "#") {
		return errors.New("--target must be an absolute HTTP or HTTPS URL with a host and without credentials, query, or fragment")
	}
	if strings.HasPrefix(upstream.Host, "[") {
		if _, err := netip.ParseAddr(upstream.Hostname()); err != nil {
			return errors.New("--target has an invalid bracketed IP address")
		}
	} else if strings.Contains(upstream.Hostname(), ":") {
		return errors.New("--target IPv6 addresses must be enclosed in brackets")
	}
	if targetPort := upstream.Port(); targetPort != "" {
		portNumber, err := strconv.ParseUint(targetPort, 10, 16)
		if err != nil || portNumber == 0 {
			return errors.New("--target port must be numeric and from 1 to 65535")
		}
	} else if strings.HasSuffix(upstream.Host, ":") {
		return errors.New("--target port must not be empty")
	}
	if strings.TrimSpace(output) == "" {
		return errors.New("--output is required and must not be blank")
	}
	return nil
}
