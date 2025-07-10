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

type ArgsAndOptions struct {
	LoggerLevel string
	LogSentMsgs bool
	LogRecvMsgs bool
	ShowPreview bool
	OutputFile  string
	TorrentFile string
}

func setupFlags() ArgsAndOptions {
	loggerLevel := flag.String("log-level", "error", "can be 'debug', 'warning', 'error' or 'none'")
	logSentMsgs := flag.Bool("sent-msg", false, "if debug is enabled, logs sent messages")
	logRecvMsgs := flag.Bool("recv-msg", false, "if debug is enabled, logs received messages")
	showTorrentPreview := flag.Bool("preview", false, "prints the information about the .torrent, without downloading anything")
	outFile := flag.String("output", "", "specify where to write the downloaded content. defaults to the name specified in the torrent file")
	
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS...] <TORRENT>\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	torrentPath := flag.Arg(0)

	return ArgsAndOptions{
		LoggerLevel: *loggerLevel,
		LogSentMsgs: *logSentMsgs,
		LogRecvMsgs: *logRecvMsgs,
		ShowPreview: *showTorrentPreview,
		OutputFile:  *outFile,
		TorrentFile: torrentPath,
	}
}

func main() {
	argsAndOptions := setupFlags()
	if argsAndOptions.TorrentFile == "" {
		fmt.Fprintf(os.Stderr, "must provide torrent file\n")
		os.Exit(1)
	}

	if err := logger.SetupLoggerOpts(
		argsAndOptions.LoggerLevel,
		argsAndOptions.LogSentMsgs,
		argsAndOptions.LogRecvMsgs,
	); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup logger: %s\n", err.Error())
		os.Exit(1)
	}

	torr, err := torrents.TorrentFromFile(argsAndOptions.TorrentFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get torrent info: %s\n", err.Error())
		os.Exit(1)
	}

	if argsAndOptions.ShowPreview {
		s, err := torr.JsonPreviewIndented()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to show torrent preview: %s\n", err.Error())
			os.Exit(1)
		}

		fmt.Printf("%s", s)
		return
	}

	of := torr.FileName
	if argsAndOptions.OutputFile != "" {
		of = argsAndOptions.OutputFile
	}

	if err := torrents.StartDownload(torr, of); err != nil {
		fmt.Fprintf(os.Stderr, "failed to download: %s\n", err.Error())
		os.Exit(1)
	}
}
