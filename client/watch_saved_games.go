// This program watches for recently changed stardew save files and publishes them to the
// Stardew Rocks AMQP server.
package main

import (
	"crypto/rand"
	"log"
	"math/big"
	"os"
	"path"
	"strings"
	"time"

	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/marcsauter/single"
	"github.com/streadway/amqp"
)

const (
	verbose = false
)

func stardewFolder() string {
	return path.Join(os.Getenv("AppData"), "StardewValley/Saves")
}

func allSaveGameInfos() ([]string, error) {
	return filepath.Glob(stardewFolder() + "/*/*")
}

func isDir(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

func relPath(p string) string {
	rel, err := filepath.Rel(stardewFolder(), p)
	if err != nil {
		log.Fatal(err)
	}
	return rel
}

func watchAndPublish(topic *amqp.Channel, cancel chan *amqp.Error) {

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	gameSaves, err := allSaveGameInfos()
	if err != nil {
		log.Fatal(err)
	}

	// Find new folders containing new saved games.
	err = watcher.Add(stardewFolder())
	if err != nil {
		log.Fatal(err)
	}

	watched := map[string]bool{}

	for _, save := range gameSaves {
		err = watcher.Add(save)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Watching %v", relPath(save))
		watched[save] = true
	}
	stop := make(chan bool, 1)

	go func() {

		for {
			select {
			case <-stop:
				return
			// The game is saved in a temporary file first (Directory/SaveGameInfo_STARDEWVALLEYTMP).
			// If the write works, it renames it to replace the older file.
			// Our job here is to watch for new files being created and written to, and stop watching the ones that get deleted.
			//
			// Sometimes game crashes happen if we get things wrong. My theory is that happens if we don't stop
			// watching the files and then they get written to again - or so.
			// If that's true, then this is all very racy :-(. If the game manages to open the new file before we
			// remove the file watch, a crash may happen.

			case event := <-watcher.Events:
				if verbose {
					log.Println("file watch:", relPath(event.Name), event.String())
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					watcher.Remove(event.Name)
					delete(watched, event.Name)
					if verbose {
						log.Println("Deleted file watch:", relPath(event.Name))
					}
				} else if !watched[event.Name] {
					watcher.Add(event.Name)
					watched[event.Name] = true
					log.Printf("Watching %v", relPath(event.Name))
				}
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Rename == fsnotify.Rename || event.Op&fsnotify.Write == fsnotify.Write {
					if verbose {
						log.Println("Found modified file:", relPath(event.Name))
					}
					switch {
					case isDir(event.Name):
						continue
					case strings.Contains(path.Base(event.Name), "SAVETMP"):
						// Ignore the TMP file, only read after it's renamed. This avoid crashes.
						continue
					case strings.Contains(path.Base(event.Name), "SaveGameInfo"):
						if err := publishSavedGame(topic, event.Name); err != nil {
							// This is normal. We tried to open the file after it's been renamed.
							if verbose {
								log.Print("could not publish new save game content:", err)
							}
							continue
						}
						log.Print("[x] New save game published")
					default:
						if err := publishOtherFiles(topic, event.Name); err != nil {
							// This is normal. We tried to open the file after it's been renamed.
							if verbose {
								log.Printf("could not publish content: %v", relPath(event.Name), err)
							}
							continue
						}
						log.Printf("[x] New detailed game file published")
					}
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()
	err = <-cancel
	log.Printf("Channel Error: %v", err)
	stop <- true
	return
}

func main() {

	s := single.New("stardew-rocks-client") // Will exit if already running.
	s.Lock()
	defer s.Unlock()
	for {

		topic, close, err := rabbitStart()
		if err != nil {
			log.Fatal(err)
		}
		defer close()

		// Find when we need to reconnect.
		cc := make(chan *amqp.Error)
		topic.NotifyClose(cc)

		watchAndPublish(topic, cc)

		// Don't retry too fast.
		time.Sleep(randSleep())
	}
}

func randSleep() time.Duration {
	n, _ := rand.Int(rand.Reader, big.NewInt(60))
	return time.Duration(n.Int64()) * time.Second
}
