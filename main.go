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

	"github.com/TatuMon/bittorrent-client/src/torrent"
)


func main() {
	torrentLocation := flag.String("torrent", "", "specify the location of the .torrent file")
	flag.Parse()

	if torrentLocation == nil || *torrentLocation == "" {
		fmt.Fprintf(os.Stderr, "must provide torrent file\n")
		os.Exit(1)
	}

	torrentFile, err := os.Open(*torrentLocation)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open torrent file: %s\n", err.Error())
		os.Exit(1)
	}

	t, err := torrent.GetTorrent(torrentFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse torrent file: %s\n", err.Error())
		os.Exit(1)
	}

	peers, err := torrent.Announce(t)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to announce to tracker: %s\n", err.Error())
		os.Exit(1)
	}

	torrent.PrintPeersJson(peers)
}
