// Copyright (C) 2019-2021 Antoine Tenart <antoine.tenart@ack.tf>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package config

import (
	"log"
	"net"
	"os"
	"regexp"
	"strings"
)

// Config holds the entire current configuration.
type Config struct {
	Routes  []*Route
}

// Route represents a route between matched domains and a backend.
type Route struct {
	Domains   []*regexp.Regexp
	// Default backend.
	Backend   *Backend
	// Backend for ACME.
	ACME      *Backend
	// Deny and Allow contain lists of IP ranges and/or addresses to
	// whitelist or blacklist for a given route. If Allow is used, all
	// addresses are then blocked by default.
	// The more specific subnet takes precedence, and Deny wins over Allow
	// in case none is more specific.
	Deny      []*net.IPNet
	Allow     []*net.IPNet
}

// Backend represents a backend and its options.
type Backend struct {
	Address   string
	// HAProxy PROXY protocol support (None, v1, v2).
	SendProxy uint
}

// SendProxy possible values.
const (
	ProxyNone = iota
	ProxyV1	  = iota
	ProxyV2   = iota
)

// Reads a configuration file and transforms it into a Config struct.
func (c *Config) ReadFile(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	l := newLexer(f)
	c.parse(parseDirective(&l))

	return nil
}

// Parses the directives generated by the parser and generate the configuration.
func (c *Config) parse(root *Directive) {
	for _, directive := range(root.Directives) {
		route := &Route{}
		c.Routes = append(c.Routes, route)

		domains := strings.Split(directive.Name, ",")
		for _, domain := range(domains) {
			rgp, err := domain2Regex(domain)
			if err != nil {
				log.Fatal("Invalid domain: " + domain)
			}

			route.Domains = append(route.Domains, rgp)
		}

		for _, dir := range(directive.Directives) {
			switch dir.Name {
			case "backend":
				if len(dir.Args) != 1 {
					log.Fatal("Invalid backend directive")
				}
				route.Backend = parseBackend(dir)
				break
			case "acme":
				if len(dir.Args) != 1 {
					log.Fatal("Invalid acme directive")
				}
				route.ACME = parseBackend(dir)
				break
			case "deny":
				if len(dir.Args) != 1 {
					log.Fatal("Invalid deny directive")
				}
				for _, subnet := range(strings.Split(dir.Args[0], ",")) {
					route.Deny = append(route.Deny, parseRange(subnet))
				}
				break
			case "allow":
				if len(dir.Args) != 1 {
					log.Fatal("Invalid allow directive")
				}
				for _, subnet := range(strings.Split(dir.Args[0], ",")) {
					route.Allow = append(route.Allow, parseRange(subnet))
				}
				break
			default:
				continue
			}
		}

		if len(route.Allow) > 0 {
			// When using the allow directive, we should block all
			// other IPs. Set Deny to match all IPs.
			_, all4, _ := net.ParseCIDR("0.0.0.0/0")
			_, all6, _ := net.ParseCIDR("::/0")
			route.Deny = append(route.Deny, all4)
			route.Deny = append(route.Deny, all6)
		}
	}
}

func parseBackend(directive *Directive) *Backend {
	backend := &Backend{
		Address: directive.Args[0],
		SendProxy: ProxyNone,
	}

	for _, d := range(directive.Directives) {
		switch d.Name {
		// HAProxy PROXY protocol (v1)
		case "send-proxy":
			if len(d.Args) > 0 {
				log.Fatal("Invalid send-proxy directive")
			}
			backend.SendProxy = ProxyV1
			break
		// HAProxy PROXY protocol (v2)
		case "send-proxy-v2":
			if len(d.Args) > 0 {
				log.Fatal("Invalid send-proxy directive")
			}
			backend.SendProxy = ProxyV2
			break
		}
	}

	return backend
}

// Converts a domain to a regexp.Regexp.
func domain2Regex(domain string) (*regexp.Regexp, error) {
	// Translate the domains into a regexp valid string.
	regex := "^"
	for _, r := range domain {
		switch r {
		case '*':
			regex += `.*`
			break
		case '.':
			regex += `\.`
			break
		default:
			regex += string(r)
		}
	}
	regex += "$"

	return regexp.Compile(regex)
}

// Parse a subnet string.
func parseRange(subnet string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(subnet)
	if err == nil {
		return ipnet
	}

	ip := net.ParseIP(subnet)
	if ip == nil {
		log.Fatal("Could not parse subnet " + subnet)
	}

	// IP is an IPv4 address, its CIDR should be /32.
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{ IP: ip, Mask: net.CIDRMask(32, 32) }
	}

	// IP is an IPv6 address, its CIDR should be /128.
	return &net.IPNet{ IP: ip, Mask: net.CIDRMask(128, 128) }
}
