package torrent

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strconv"
)

// TODO
// When parsing, verify the usage of network byte order
type Peer struct {
	IP   string
	Port int
}

type TrackerResponse struct {
	FailureReason  string
	WarningMessage string
	Interval       uint
	MinInterval    uint
	TrackerID      string
	Seeders        uint
	Leechers       uint
	Peers
}

// Generated only once
var peerID string

func genPeerID() {
	prefix := []byte("-TM0001-")

	randSlice := make([]byte, 12)
	_, _ = rand.Read(randSlice)

	peerID = string(append(prefix, randSlice...))
}

func GetPeerID() string {
	if len(peerID) == 0 {
		genPeerID()
		return peerID
	}

	return peerID
}

// Don't know where to get the port yet
func GetTrackerPort() uint {
	return 6881
}

func GetTrackerURL(torr *Torrent) (string, error) {
	baseURL, err := url.Parse(torr.Announce)
	if err != nil {
		return "", fmt.Errorf("failed to generate URL: %w", err)
	}

	qParams := url.Values{
		"info_hash":  []string{string(torr.InfoHash[:])},
		"peer_id":    []string{GetPeerID()},
		"port":       []string{strconv.Itoa(int(GetTrackerPort()))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"left":       []string{strconv.Itoa(int(torr.FileSize))},
		"compact":    []string{"1"},
	}

	baseURL.RawQuery = qParams.Encode()
	return baseURL.String(), nil
}
