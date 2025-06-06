package torrents

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/jackpal/bencode-go"
)

type trackerResponse struct {
	FailureReason  string `bencode:"failure reason"`
	WarningMessage string `bencode:"warning message"`
	Interval       uint   `bencode:"interval"`
	MinInterval    uint   `bencode:"min interval"`
	TrackerID      string `bencode:"tracker id"`
	Complete       uint   `bencode:"complete"`   // aka seeders
	Incomplete     uint   `bencode:"incomplete"` // aka leechers
	Peers          string `bencode:"peers"` // string of bytes
}

func trackerResponseFromBody(body io.ReadCloser) (*trackerResponse, error) {
	t := trackerResponse{}

	if err := bencode.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("failed to parse tracker response: %w", err)
	}

	return &t, nil
}

// Don't know where to get the port yet
func getTrackerPort() uint {
	return 6881
}

func getTrackerURL(torr *Torrent) (string, error) {
	baseURL, err := url.Parse(torr.Announce)
	if err != nil {
		return "", fmt.Errorf("failed to generate URL: %w", err)
	}

	qParams := url.Values{
		"info_hash":  []string{string(torr.InfoHash[:])},
		"peer_id":    []string{getClientPeerID()},
		"port":       []string{strconv.Itoa(int(getTrackerPort()))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"left":       []string{strconv.Itoa(int(torr.FileSize))},
		"compact":    []string{"1"},
	}

	baseURL.RawQuery = qParams.Encode()
	return baseURL.String(), nil
}

func announce(torr *Torrent) ([]Peer, error) {
	trackerUrl, err := getTrackerURL(torr)
	if err != nil {
		return nil, fmt.Errorf("failed to get tracker url: %w", err)
	}

	res, err := http.Get(trackerUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to tracker: %w", err)
	}

	if res.StatusCode >= 300 {
		return nil, errors.New(fmt.Sprintf("connection to tracker failed with status %d", res.StatusCode))
	}

	trackerRes, err := trackerResponseFromBody(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tracker response: %w", err)
	}
	res.Body.Close()

	if len(trackerRes.FailureReason) > 0 {
		return nil, errors.New(fmt.Sprintf("tracker responded with failure: %s", trackerRes.FailureReason))
	}

	if len(trackerRes.WarningMessage) > 0 {
		fmt.Fprintf(os.Stderr, "[TRACKER WARNING] %s", trackerRes.WarningMessage)
	}

	peers, err := peersFromTrackerResponse(trackerRes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse peers list: %w", err)
	}

	return peers, nil
}
