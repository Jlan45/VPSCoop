package sshclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vpscoop/internal/store"
)

// CarrierRoute describes the detected route and latency to a major Chinese carrier.
type CarrierRoute struct {
	Carrier   string   `json:"carrier"`
	Target    string   `json:"target"`
	LineType  string   `json:"lineType"`
	LatencyMs float64  `json:"latencyMs,omitempty"`
	Hops      []string `json:"hops"`
}

// NetworkReport holds outbound IP, geo/ASN, and 3-carrier route detection results.
type NetworkReport struct {
	IPv4    string         `json:"ipv4"`
	IPv6    string         `json:"ipv6"`
	Country string         `json:"country"`
	Region  string         `json:"region"`
	City    string         `json:"city"`
	ISP     string         `json:"isp"`
	ASN     string         `json:"asn"`
	Routes  []CarrierRoute `json:"routes"`
}

var ipRegex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
var pingAvgRegex = regexp.MustCompile(`= [\d.]+/([\d.]+)/`)

// ProbeNetwork connects to host over SSH and runs a lightweight outbound IP + 3-carrier route probe.
func ProbeNetwork(host store.HostWithSecrets) (NetworkReport, error) {
	client, err := Dial(host)
	if err != nil {
		return NetworkReport{}, err
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return NetworkReport{}, err
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout

	// Lightweight, read-only script:
	// 1. Outbound IPv4 + Geo/ASN via ip-api.com (fallback ipinfo.io)
	// 2. Outbound IPv6 via api64.ipify.org
	// 3. Traceroute/tracepath + ping to CT (202.96.209.133), CU (210.22.97.1), CM (211.136.112.200)
	const script = `
echo "===GEO==="
curl -4 -fsSL --max-time 4 "http://ip-api.com/json/?fields=status,country,regionName,city,isp,org,as,query" 2>/dev/null || echo "{}"
echo ""
echo "===IPV6==="
curl -6 -fsSL --max-time 3 "https://api64.ipify.org" 2>/dev/null || true
echo ""

trace_target() {
  LABEL=$1
  IP=$2
  OUT="/tmp/.vpsc_net_$$_${LABEL}"
  {
    echo "===ROUTE:$LABEL:$IP==="
    ping -4 -c 2 -W 1 "$IP" 2>/dev/null | tail -n 2 || true
    if command -v traceroute >/dev/null 2>&1; then
      traceroute -n -m 16 -w 1 -q 1 "$IP" 2>/dev/null || true
    elif command -v tracepath >/dev/null 2>&1; then
      tracepath -n -m 16 "$IP" 2>/dev/null || true
    else
      for ttl in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16; do
        ping -4 -n -c 1 -W 1 -t "$ttl" "$IP" 2>/dev/null | grep -E "From |from " > "${OUT}_$ttl" &
      done
      wait
      for ttl in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16; do
        cat "${OUT}_$ttl" 2>/dev/null || true
        rm -f "${OUT}_$ttl"
      done
    fi
  } > "$OUT"
}

trace_target "CT" "202.96.209.133" &
trace_target "CU" "210.22.97.1" &
trace_target "CM" "211.136.112.200" &
wait

for L in CT CU CM; do
  cat "/tmp/.vpsc_net_$$_${L}" 2>/dev/null || true
  rm -f "/tmp/.vpsc_net_$$_${L}"
done
`

	done := make(chan error, 1)
	go func() {
		done <- sess.Run(script)
	}()

	select {
	case runErr := <-done:
		if runErr != nil && stdout.Len() == 0 {
			return NetworkReport{}, fmt.Errorf("network probe failed: %w", runErr)
		}
	case <-time.After(22 * time.Second):
		_ = sess.Close()
		if stdout.Len() == 0 {
			return NetworkReport{}, fmt.Errorf("network probe timed out")
		}
	}

	return parseNetworkOutput(stdout.String()), nil
}

func parseNetworkOutput(raw string) NetworkReport {
	var rep NetworkReport
	sections := strings.Split(raw, "===")

	routeMap := map[string]CarrierRoute{}
	for i := 0; i+1 < len(sections); i++ {
		header := strings.TrimSpace(sections[i])
		body := strings.TrimSpace(sections[i+1])
		switch {
		case header == "GEO":
			var geo struct {
				Status     string `json:"status"`
				Country    string `json:"country"`
				RegionName string `json:"regionName"`
				City       string `json:"city"`
				ISP        string `json:"isp"`
				Org        string `json:"org"`
				AS         string `json:"as"`
				Query      string `json:"query"`
			}
			if json.Unmarshal([]byte(body), &geo) == nil && geo.Query != "" {
				rep.IPv4 = geo.Query
				rep.Country = geo.Country
				rep.Region = geo.RegionName
				rep.City = geo.City
				rep.ISP = geo.ISP
				if rep.ISP == "" {
					rep.ISP = geo.Org
				}
				rep.ASN = geo.AS
			}
		case header == "IPV6":
			if strings.Contains(body, ":") && !strings.Contains(body, " ") {
				rep.IPv6 = body
			}
		case strings.HasPrefix(header, "ROUTE:"):
			parts := strings.SplitN(header, ":", 3)
			if len(parts) != 3 {
				continue
			}
			code, target := parts[1], parts[2]
			carrierName := map[string]string{
				"CT": "中国电信",
				"CU": "中国联通",
				"CM": "中国移动",
			}[code]
			if carrierName == "" {
				carrierName = code
			}

			var latency float64
			if m := pingAvgRegex.FindStringSubmatch(body); len(m) == 2 {
				latency, _ = strconv.ParseFloat(m[1], 64)
			}

			seen := map[string]bool{}
			var hops []string
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "traceroute to") || strings.HasPrefix(line, "PING ") {
					continue
				}
				if ip := ipRegex.FindString(line); ip != "" && ip != target && !seen[ip] {
					seen[ip] = true
					hops = append(hops, ip)
				}
			}

			routeMap[code] = CarrierRoute{
				Carrier:   carrierName,
				Target:    target,
				LineType:  classifyRoute(code, hops),
				LatencyMs: latency,
				Hops:      hops,
			}
		}
	}

	for _, code := range []string{"CT", "CU", "CM"} {
		if r, ok := routeMap[code]; ok {
			rep.Routes = append(rep.Routes, r)
		}
	}
	return rep
}

func classifyRoute(carrier string, hops []string) string {
	has5943 := false
	has20297 := false
	has9929 := false
	has4837 := false
	hasCMIN2 := false
	hasCMNET := false

	for _, ip := range hops {
		switch {
		case strings.HasPrefix(ip, "59.43."):
			has5943 = true
		case strings.HasPrefix(ip, "202.97."):
			has20297 = true
		case strings.HasPrefix(ip, "218.105.") || strings.HasPrefix(ip, "210.51."):
			has9929 = true
		case strings.HasPrefix(ip, "219.158."):
			has4837 = true
		case strings.HasPrefix(ip, "223.120."):
			hasCMIN2 = true
		case strings.HasPrefix(ip, "221.176.") || strings.HasPrefix(ip, "221.183."):
			hasCMNET = true
		}
	}

	switch carrier {
	case "CT":
		if has5943 && !has20297 {
			return "电信 CN2 GIA"
		}
		if has5943 && has20297 {
			return "电信 CN2 GT"
		}
		if has20297 {
			return "电信 163 骨干网"
		}
	case "CU":
		if has9929 {
			return "联通 AS9929"
		}
		if has4837 {
			return "联通 AS4837"
		}
	case "CM":
		if hasCMIN2 {
			return "移动 CMI / CMIN2"
		}
		if hasCMNET {
			return "移动 CMNET"
		}
	}

	if len(hops) == 0 {
		return "未检测到跳数 (无 traceroute/tracepath)"
	}
	return "普通直连 / 国际路由"
}

// BuildRouteTags extracts concise, searchable tags from a NetworkReport.
func BuildRouteTags(rep NetworkReport) []string {
	var tags []string
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}

	if rep.IPv6 != "" {
		add("IPv6")
	}
	for _, r := range rep.Routes {
		switch r.LineType {
		case "电信 CN2 GIA":
			add("CN2-GIA")
		case "电信 CN2 GT":
			add("CN2-GT")
		case "电信 163 骨干网":
			add("163")
		case "联通 AS9929":
			add("AS9929")
		case "联通 AS4837":
			add("AS4837")
		case "移动 CMI / CMIN2":
			add("CMIN2")
		case "移动 CMNET":
			add("CMNET")
		}
	}
	return tags
}
