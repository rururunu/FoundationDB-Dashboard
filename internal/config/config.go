// Package config reads how this process should listen and which FoundationDB
// cluster it should open.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const DefaultPort = 8080

// Config is the process configuration. ClusterFile and ConnectionString are
// alternatives: a cluster file path wins when both are set.
type Config struct {
	Addr              string
	ClusterFile       string
	ConnectionString  string
	ConnectionDisplay string
	TrendFile         string
	ClustersFile      string
	AuthEnabled       bool
	AuthUser          string
	AuthPassword      string
}

// FromEnv reads PORT, FDB_CLUSTER_FILE, and FDB_CONNECTION_STRING.
func FromEnv() (Config, error) {
	file := strings.TrimSpace(os.Getenv("FDB_CLUSTER_FILE"))
	conn := strings.TrimSpace(os.Getenv("FDB_CONNECTION_STRING"))
	if file == "" && conn == "" {
		return Config{}, errors.New("set FDB_CLUSTER_FILE or FDB_CONNECTION_STRING")
	}
	if strings.ContainsAny(conn, "\r\n") {
		return Config{}, errors.New("FDB_CONNECTION_STRING must be a single line")
	}

	port := DefaultPort
	if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("PORT must be 1-65535, got %q", raw)
		}
		port = n
	}

	authOn, authUser, authPassword, err := authFromEnv()
	if err != nil {
		return Config{}, err
	}

	display := conn
	if file != "" {
		display = file
		if text, err := os.ReadFile(file); err == nil {
			line := strings.TrimSpace(string(text))
			if i := strings.IndexAny(line, "\r\n"); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if line != "" {
				display = line
			}
		}
	}

	return Config{
		Addr:              fmt.Sprintf("0.0.0.0:%d", port),
		ClusterFile:       file,
		ConnectionString:  conn,
		ConnectionDisplay: display,
		TrendFile:         strings.TrimSpace(os.Getenv("TREND_FILE")),
		ClustersFile:      strings.TrimSpace(os.Getenv("CLUSTERS_FILE")),
		AuthEnabled:       authOn,
		AuthUser:          authUser,
		AuthPassword:      authPassword,
	}, nil
}

func authFromEnv() (bool, string, string, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AUTH"))) {
	case "", "0", "off", "false", "no":
		return false, "", "", nil
	case "1", "on", "true", "yes":
	default:
		return false, "", "", fmt.Errorf("AUTH must be on or off, got %q", os.Getenv("AUTH"))
	}
	user := strings.TrimSpace(os.Getenv("AUTH_USER"))
	password := os.Getenv("AUTH_PASSWORD")
	if user == "" || strings.TrimSpace(password) == "" {
		return false, "", "", errors.New("AUTH=on requires AUTH_USER and AUTH_PASSWORD")
	}
	if strings.ContainsAny(user, "\r\n") || strings.ContainsAny(password, "\r\n") {
		return false, "", "", errors.New("AUTH_USER and AUTH_PASSWORD must be a single line")
	}
	if len(user) > 64 || len(password) > 256 {
		return false, "", "", errors.New("AUTH_USER or AUTH_PASSWORD is too long")
	}
	return true, user, password, nil
}
