package importer

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"vpscoop/internal/store"
)

// ParseAuto detects the format (json, csv, or ssh_config) and parses host entries.
func ParseAuto(format, content string) ([]store.HostInput, string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, "", fmt.Errorf("import content is empty")
	}

	fmtMode := strings.ToLower(strings.TrimSpace(format))
	if fmtMode == "" || fmtMode == "auto" {
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			fmtMode = "json"
		} else if looksLikeCSV(trimmed) {
			fmtMode = "csv"
		} else {
			fmtMode = "ssh_config"
		}
	}

	switch fmtMode {
	case "json":
		hosts, err := parseJSON(trimmed)
		return hosts, "json", err
	case "csv":
		hosts, err := parseCSV(trimmed)
		return hosts, "csv", err
	case "ssh_config", "sshconfig":
		hosts, err := parseSSHConfig(trimmed)
		return hosts, "ssh_config", err
	default:
		return nil, "", fmt.Errorf("unsupported import format %q", format)
	}
}

func looksLikeCSV(s string) bool {
	firstLine, _, _ := strings.Cut(s, "\n")
	lower := strings.ToLower(firstLine)
	return strings.Contains(firstLine, ",") &&
		(strings.Contains(lower, "host") ||
			strings.Contains(lower, "ip") ||
			strings.Contains(lower, "label") ||
			strings.Contains(lower, "name") ||
			strings.Contains(lower, "address"))
}

func parseSSHConfig(content string) ([]store.HostInput, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var results []store.HostInput
	var current *store.HostInput
	var currentAliases []string

	flush := func() {
		if current == nil {
			return
		}
		for _, alias := range currentAliases {
			if alias == "*" || strings.ContainsAny(alias, "*?") {
				continue
			}
			item := *current
			item.Name = alias
			if item.Host == "" {
				item.Host = alias
			}
			if item.Port <= 0 {
				item.Port = 22
			}
			if item.Username == "" {
				item.Username = "root"
			}
			results = append(results, item)
		}
		current = nil
		currentAliases = nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip inline comment
		if idx := strings.Index(line, " #"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		val := strings.Join(fields[1:], " ")
		val = strings.Trim(val, `"'`)

		switch key {
		case "host":
			flush()
			current = &store.HostInput{
				Port:         22,
				Username:     "root",
				AuthType:     "password",
				Currency:     "USD",
				BillingCycle: "monthly",
			}
			currentAliases = fields[1:]
		case "hostname":
			if current != nil {
				current.Host = val
			}
		case "port":
			if current != nil {
				if p, err := strconv.Atoi(val); err == nil && p > 0 && p <= 65535 {
					current.Port = p
				}
			}
		case "user":
			if current != nil {
				current.Username = val
			}
		case "identityfile":
			if current != nil {
				current.AuthType = "key"
				if current.Notes == "" {
					current.Notes = "IdentityFile: " + val
				} else {
					current.Notes += "\nIdentityFile: " + val
				}
			}
		}
	}
	flush()

	if len(results) == 0 {
		return nil, fmt.Errorf("no valid Host entries found in SSH config")
	}
	return results, nil
}

func parseCSV(content string) ([]store.HostInput, error) {
	r := csv.NewReader(strings.NewReader(content))
	r.TrimLeadingSpace = true
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}

	colIdx := map[string]int{}
	for i, h := range headers {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	get := func(row []string, names ...string) string {
		for _, n := range names {
			if idx, ok := colIdx[n]; ok && idx < len(row) {
				if v := strings.TrimSpace(row[idx]); v != "" {
					return v
				}
			}
		}
		return ""
	}

	var results []store.HostInput
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv row: %w", err)
		}

		host := get(row, "hostname", "host", "ip", "address", "hostname/ip", "server")
		name := get(row, "label", "name", "title", "alias", "名称", "主机名")
		if name == "" {
			name = host
		}
		if host == "" && name == "" {
			continue
		}
		if host == "" {
			host = name
		}

		port := 22
		if pStr := get(row, "port", "端口"); pStr != "" {
			if p, pErr := strconv.Atoi(pStr); pErr == nil && p > 0 && p <= 65535 {
				port = p
			}
		}

		user := get(row, "username", "user", "用户名")
		if user == "" {
			user = "root"
		}

		cost := 0.0
		if cStr := get(row, "cost", "price", "花费"); cStr != "" {
			cost, _ = strconv.ParseFloat(cStr, 64)
		}

		var tags []string
		if tStr := get(row, "tags", "标签"); tStr != "" {
			for _, t := range strings.Split(tStr, ";") {
				for _, sub := range strings.Split(t, ",") {
					if s := strings.TrimSpace(sub); s != "" {
						tags = append(tags, s)
					}
				}
			}
		}

		results = append(results, store.HostInput{
			Name:         name,
			GroupName:    get(row, "group", "groupname", "group_name", "分组"),
			Provider:     get(row, "provider", "vendor", "购买商"),
			Region:       get(row, "region", "location", "地区"),
			Host:         host,
			Port:         port,
			Username:     user,
			AuthType:     "password",
			Password:     get(row, "password", "密码"),
			Cost:         cost,
			Currency:     get(row, "currency", "货币"),
			BillingCycle: get(row, "billingcycle", "billing_cycle", "cycle", "计费周期"),
			ExpiresAt:    get(row, "expiresat", "expires_at", "expiry", "到期时间"),
			Specs:        get(row, "specs", "配置"),
			Tags:         tags,
			Notes:        get(row, "notes", "note", "comment", "备注"),
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no valid host rows found in CSV")
	}
	return results, nil
}

func parseJSON(content string) ([]store.HostInput, error) {
	var rawItems []map[string]any

	if strings.HasPrefix(content, "[") {
		if err := json.Unmarshal([]byte(content), &rawItems); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
	} else {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal([]byte(content), &wrapper); err != nil {
			return nil, fmt.Errorf("invalid JSON object: %w", err)
		}
		for _, k := range []string{"hosts", "Hosts", "data", "items"} {
			if raw, ok := wrapper[k]; ok {
				if err := json.Unmarshal(raw, &rawItems); err == nil {
					break
				}
			}
		}
		if len(rawItems) == 0 {
			var single map[string]any
			if err := json.Unmarshal([]byte(content), &single); err == nil {
				rawItems = []map[string]any{single}
			}
		}
	}

	strVal := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k]; ok && v != nil {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		return ""
	}

	var results []store.HostInput
	for _, m := range rawItems {
		host := strVal(m, "host", "hostname", "address", "ip", "Address", "Hostname")
		name := strVal(m, "name", "label", "title", "Label", "Name")
		if name == "" {
			name = host
		}
		if host == "" {
			host = name
		}
		if host == "" {
			continue
		}

		port := 22
		for _, pk := range []string{"port", "Port", "sshPort"} {
			if v, ok := m[pk]; ok && v != nil {
				switch n := v.(type) {
				case float64:
					if n > 0 && n <= 65535 {
						port = int(n)
					}
				case string:
					if p, err := strconv.Atoi(n); err == nil && p > 0 && p <= 65535 {
						port = p
					}
				}
				break
			}
		}

		user := strVal(m, "username", "user", "Username", "User")
		if user == "" {
			user = "root"
		}

		cost := 0.0
		if v, ok := m["cost"]; ok {
			if f, ok := v.(float64); ok {
				cost = f
			}
		}

		results = append(results, store.HostInput{
			Name:         name,
			GroupName:    strVal(m, "groupName", "group", "Group"),
			Provider:     strVal(m, "provider", "vendor", "Provider"),
			Region:       strVal(m, "region", "location", "Region"),
			Host:         host,
			Port:         port,
			Username:     user,
			AuthType:     "password",
			Password:     strVal(m, "password", "Password"),
			Cost:         cost,
			Currency:     strVal(m, "currency"),
			BillingCycle: strVal(m, "billingCycle", "billing_cycle"),
			ExpiresAt:    strVal(m, "expiresAt", "expires_at"),
			Specs:        strVal(m, "specs"),
			Notes:        strVal(m, "notes", "note", "comment", "Notes"),
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no valid host objects found in JSON")
	}
	return results, nil
}
