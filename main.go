/**
This project is being made according to the documentation written in these posts:
https://1blog.jse.li/posts/torrent/
https://wiki.theory.org/BitTorrentSpecification#Metainfo_File_Structure
*/

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TatuMon/bittorrent-client/src/torrents"
	"github.com/sirupsen/logrus"
)


func main() {
	showDebugLogs := flag.Bool("debug", false, "show debug logs")
	torrentPath := flag.String("torrent", "", "specify the location of the .torrent file")
	flag.Parse()

	if *showDebugLogs {
		logrus.SetLevel(logrus.DebugLevel)
	}

	if torrentPath == nil || *torrentPath == "" {
		fmt.Fprintf(os.Stderr, "must provide torrent file\n")
		os.Exit(1)
	}

	torr, err := torrents.TorrentFromFile(*torrentPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get torrent info: %s\n", err.Error())
		os.Exit(1)
	}

	peers, err := torrents.Announce(torr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to announce to tracker: %s\n", err.Error())
		os.Exit(1)
	}

	torrents.ConnectToPeersAsync(torr, peers)
}
