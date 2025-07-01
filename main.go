/**
This project is being made according to the documentation written in these posts:
https://www.bittorrent.org/beps/bep_0003.html
https://1blog.jse.li/posts/torrent/
https://wiki.theory.org/BitTorrentSpecification#Metainfo_File_Structure
*/

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TatuMon/bittorrent-client/logger"
	"github.com/TatuMon/bittorrent-client/src/torrents"
)


func main() {
	loggerLevel := flag.String("log-level", "error", "can be 'debug', 'warning', 'error' or 'none'")
	logSentMsgs := flag.Bool("sent-msg", false, "if debug is enabled, logs sent messages")
	logRecvMsgs := flag.Bool("recv-msg", false, "if debug is enabled, logs received messages")
	torrentPath := flag.String("torrent", "", "specify the location of the .torrent file")
	showTorrentPreview := flag.Bool("preview", false, "prints the information about the .torrent, without downloading anything")
	flag.Parse()

	if err := logger.SetupLoggerOpts(*loggerLevel, *logSentMsgs, *logRecvMsgs); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup logger: %s\n", err.Error())
		os.Exit(1)
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

	if *showTorrentPreview {
		s, err := torr.JsonPreviewIndented()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to show torrent preview: %s\n", err.Error())
			os.Exit(1)
		}

		fmt.Printf("%s", s)
		return
	}

	torrents.StartDownload(torr)
}
