//go:build darwin

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsevents"
)

// Installing launchd service:
// sudo cp /Users/alan/Documents/git/fsevents/example/filefind.plist /Library/LaunchAgents/filefind.plist
// sudo chmod 644 /Library/LaunchDaemons/com.example.filefind.plist
// sudo chown root:wheel /Library/LaunchDaemons/com.example.filefind.plist
// sudo launchctl bootstrap system /Library/LaunchDaemons/com.example.filefind.plist
// sudo launchctl bootout system /Library/LaunchDaemons/com.example.filefind.plist

// move the exmaple program to a new cmd
// convert db.go to a package to share
// open sqlite db
// 		todo: read historical events
// Parse incoming events
// if rename:
// 		check file exists
//		if so, it was the target
//		if not, it was the source and needs to be deleted from the db
// if create, delete:
//		add or remove from the db
//
// todo:
//		create installer for launchd service
//

func main() {
	// path, err := os.MkdirTemp("", "fsexample")
	// if err != nil {
	// 	log.Fatalf("Failed to create MkdirTemp: %v", err)
	// }
	path := flag.String("path", "/", "Path to watch for file system events")
	flag.Parse()

	if *path == "" {
		log.Fatal("Path is required")
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)

	go func() {
		sig := <-c
		fmt.Println("Received SIGTERM:", sig)
		// Perform cleanup or graceful shutdown here
		os.Exit(0)
	}()

	dev, err := fsevents.DeviceForPath(*path)
	if err != nil {
		log.Fatalf("Failed to retrieve device for path: %v", err)
	}

	log.Print("Monitoring path: ", *path)
	log.Print(dev)
	log.Println(fsevents.EventIDForDeviceBeforeTime(dev, time.Now()))

	es := &fsevents.EventStream{
		Paths:   []string{*path},
		Latency: 500 * time.Millisecond,
		Device:  dev,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot}
	es.Start()
	ec := es.Events

	log.Println("Device UUID", fsevents.GetDeviceUUID(dev))

	go func() {
		for msg := range ec {
			for _, event := range msg {
				logEvent(event)
			}
		}
	}()

	// Keep the program running
	select {}

	// 	in := bufio.NewReader(os.Stdin)

	// 	if false {
	// 		log.Print("Started, press enter to GC")
	// 		in.ReadString('\n')
	// 		runtime.GC()
	// 		log.Print("GC'd, press enter to quit")
	// 		in.ReadString('\n')
	// 	} else {
	// 		log.Print("Started, press enter to stop")
	// 		in.ReadString('\n')
	// 		es.Stop()

	// 		log.Print("Stopped, press enter to restart")
	// 		in.ReadString('\n')
	// 		es.Resume = true
	// 		es.Start()

	//		log.Print("Restarted, press enter to quit")
	//		in.ReadString('\n')
	//		es.Stop()
	//	}
}

var noteDescription = map[fsevents.EventFlags]string{
	fsevents.MustScanSubDirs: "MustScanSubdirs",
	fsevents.UserDropped:     "UserDropped",
	fsevents.KernelDropped:   "KernelDropped",
	fsevents.EventIDsWrapped: "EventIDsWrapped",
	fsevents.HistoryDone:     "HistoryDone",
	fsevents.RootChanged:     "RootChanged",
	fsevents.Mount:           "Mount",
	fsevents.Unmount:         "Unmount",

	fsevents.ItemCreated:       "Created",
	fsevents.ItemRemoved:       "Removed",
	fsevents.ItemInodeMetaMod:  "InodeMetaMod",
	fsevents.ItemRenamed:       "Renamed",
	fsevents.ItemModified:      "Modified",
	fsevents.ItemFinderInfoMod: "FinderInfoMod",
	fsevents.ItemChangeOwner:   "ChangeOwner",
	fsevents.ItemXattrMod:      "XAttrMod",
	fsevents.ItemIsFile:        "IsFile",
	fsevents.ItemIsDir:         "IsDir",
	fsevents.ItemIsSymlink:     "IsSymLink",
}

func logEvent(event fsevents.Event) {
	note := ""
	for bit, description := range noteDescription {
		if event.Flags&bit == bit {
			note += description + " "
		}
	}
	log.Printf("EventID: %d Path: %s Flags: %s", event.ID, event.Path, note)
}
