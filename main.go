package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	cors "gopkg.in/gin-contrib/cors.v1"

	"github.com/gin-gonic/gin"
	"github.com/grandcat/zeroconf"
	"github.com/sirupsen/logrus"
)

const (
	version     = "0.0.5b2"
	defaultPort = 8080

	zeroConfName    = "OMX Remote"
	zeroConfService = "_omx-remote-api._tcp"
	zeroConfDomain  = "local."
)

var (
	// Commands mapping to control OMXPlayer, these are piped via STDIN to omxplayer process
	Commands = map[string]string{
		"pause":             "p",            // Pause/continue playback
		"stop":              "q",            // Stop playback and exit
		"volume_up":         "+",            // Change volume by +3dB
		"volume_down":       "-",            // Change volume by -3dB
		"subtitles":         "s",            // Enable/disable subtitles
		"seek_back":         "\x1b\x5b\x44", // Seek -30 seconds
		"seek_back_fast":    "\x1b\x5b\x42", // Seek -600 second
		"seek_forward":      "\x1b\x5b\x43", // Seek +30 second
		"seek_forward_fast": "\x1b\x5b\x41", // Seek +600 seconds
	}

	// OmxPath is path to omxplayer executable
	OmxPath string

	// Omx is a child process for spawning omxplayer
	Omx *exec.Cmd

	// OmxIn is a child process STDIN pipe to send commands
	OmxIn io.WriteCloser

	// OmxOut is a child process STDOUT pipe to read status
	OmxOut io.ReadCloser

	// Command is a channel to pass along commands to the player routine
	Command chan string

	// StatusStream channel to broadcast any changes in playing media via SSE
	StatusStream = make(chan *MediaEntry)

	// PlayingMedia represents currently playing media
	PlayingMedia *MediaEntry

	// PlayList is a list of media entries to play sequentially
	PlayList *PList

	// LOG is a global app logger
	LOG *logrus.Logger
)

// MediaEntry describes model of currently playable video.
type MediaEntry struct {
	RawURL    string                 `json:"url,omitempty"`
	MediaInfo map[string]interface{} `json:"media_info,omitempty"`
}

// APIErr is a generic structure for all errors returned from API
type APIErr struct {
	Message string `json:"message,omitempty"`
}

// PList holds the list of media items with pointer to the playing one
type PList struct {
	CurrentIndex int
	Entries      []*MediaEntry
}

// Next move pointer to a current element to the next element in the list and
// returns the media entry
func (pl *PList) Next() *MediaEntry {
	if len(pl.Entries) == 0 {
		return nil
	}

	nextIndex := pl.CurrentIndex + 1
	if len(pl.Entries) < nextIndex+1 {
		return nil
	}

	pl.CurrentIndex = nextIndex
	return pl.Entries[nextIndex]
}

// Select move pointer to a current element to the specific element refered by its index
// and return the media entry
func (pl *PList) Select(position int) *MediaEntry {
	plistSize := len(pl.Entries)
	if plistSize == 0 {
		return nil
	}

	if plistSize < position {
		return nil
	}

	pl.CurrentIndex = position
	return pl.Entries[position]
}

// AddEntry adds a new media entry to the end of the playlist.
func (pl *PList) AddEntry(entry *MediaEntry) {
	pl.Entries = append(pl.Entries, entry)
}

func nextToPlay() *MediaEntry {
	if PlayList == nil {
		return nil
	}

	return PlayList.Next()
}

// Determine the full path to omxplayer executable. Returns error if not found.
func omxDetect() error {
	buff, err := exec.Command("which", "omxplayer").Output()
	if err != nil {
		return err
	}

	// Set path in global variable
	OmxPath = strings.TrimSpace(string(buff))

	return nil
}

// Start command listener. Commands are coming in through a channel.
func omxListen() {
	Command = make(chan string)

	for {
		command := <-Command

		// Skip command handling of omx player is not active
		if Omx == nil {
			continue
		}

		// Send command to the player
		omxWrite(command)

		// Attempt to kill the process if stop command is requested
		if command == "stop" {
			omxStop()
		}

		broadcastStatus()
	}
}

// Start omxplayer playback for a given video file. Returns error if start fails.
func omxPlay(c MediaEntry) error {
	contentURL, err := url.Parse(c.RawURL)
	if err != nil {
		return err
	}

	Omx = exec.Command(
		OmxPath,             // path to omxplayer executable
		"--blank",           // set background to black
		"--adev",            // audio out device
		"hdmi",              // using hdmi for audio/video
		contentURL.String(), // path to video file
	)

	// Grab child process STDIN
	stdin, err := Omx.StdinPipe()
	if err != nil {
		return err
	}

	defer stdin.Close()

	// Grab child process STDOUT
	stdout, err := Omx.StdoutPipe()
	if err != nil {
		return err
	}

	defer stdout.Close()

	// Start omxplayer execution.
	// If successful, something will appear on HDMI display.
	err = Omx.Start()
	if err != nil {
		return err
	}

	// Set current file
	PlayingMedia = &c

	broadcastStatus()

	// Make child's STDIN globally available
	OmxIn = stdin
	OmxOut = stdout

	// Wait until child process is finished
	err = Omx.Wait()
	if err != nil {
		LOG.Error(fmt.Sprintln("Process exited with error:", err))
	} else {
		LOG.Info("Process exited without errors.")
	}

	omxCleanup()

	broadcastStatus()

	if next := nextToPlay(); next != nil {
		go omxPlay(*next)
	}

	return nil
}

// Write a command string to the omxplayer process's STDIN
func omxWrite(command string) {
	if OmxIn != nil {
		LOG.Debug("Write omx command: " + command)
		n, err := io.WriteString(OmxIn, Commands[command])
		if err != nil {
			LOG.Error(err.Error())
			return
		}

		LOG.Debug(fmt.Sprintf("%d bytes succsessfully written", n))
	}
}

func omxStop() {
	if !omxIsActive() {
		return
	}

	err := Omx.Process.Kill()
	if err != nil {
		LOG.Error(err.Error())
	}
	omxCleanup()
}

// Terminate any running omxplayer processes. Fixes random hangs.
func omxKill() {
	exec.Command("killall", "omxplayer.bin").Output()
	exec.Command("killall", "omxplayer").Output()
}

// Reset internal state and stop any running processes
func omxCleanup() {
	Omx = nil
	OmxIn = nil
	PlayingMedia = nil

	omxKill()
}

// Check if player is currently active
func omxIsActive() bool {
	return Omx != nil
}

func httpCommand(c *gin.Context) {
	val := c.Params.ByName("command")

	if _, ok := Commands[val]; !ok {
		c.JSON(http.StatusBadRequest, APIErr{"Invalid command"})
		return
	}

	// Handle requested commmand
	Command <- val

	c.Status(http.StatusAccepted)
}

func httpPlay(c *gin.Context) {
	if omxIsActive() {
		c.JSON(http.StatusBadRequest, APIErr{"Player is already running"})
		return
	}

	media := MediaEntry{}
	err := c.BindJSON(&media)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIErr{err.Error()})
		return
	}

	go omxPlay(media)

	c.Status(http.StatusAccepted)
}

func httpStatus(c *gin.Context) {
	result := struct {
		Running    bool        `json:"running"`
		MediaEntry *MediaEntry `json:"entry,omitempty"`
		PlayList   *PList      `json:"playlist,omitempty"`
	}{
		Running:    omxIsActive(),
		MediaEntry: PlayingMedia,
		PlayList:   PlayList,
	}

	c.JSON(http.StatusOK, result)
}

func httpNewPList(c *gin.Context) {
	body := &struct {
		Entries []*MediaEntry `json:"entries,omitempty"`
	}{}

	err := c.BindJSON(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIErr{err.Error()})
		return
	}

	PlayList = &PList{Entries: body.Entries, CurrentIndex: -1}
}

func httpPListNext(c *gin.Context) {
	omxStop()

	mi := PlayList.Next()
	if mi == nil {
		c.JSON(http.StatusNoContent, nil)
		return
	}

	go omxPlay(*mi)

	c.JSON(http.StatusOK, mi)
}

func httpPListSelect(c *gin.Context) {
	omxStop()

	body := &struct {
		Position int `json:"position,omitempty"`
	}{}

	err := c.BindJSON(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIErr{Message: err.Error()})
		return
	}

	mi := PlayList.Select(body.Position)
	if mi == nil {
		c.JSON(http.StatusNoContent, nil)
		return
	}

	go omxPlay(*mi)

	c.JSON(http.StatusOK, mi)
}

func httpPListAddEntry(c *gin.Context) {
	entry := &MediaEntry{}
	err := c.BindJSON(entry)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIErr{Message: err.Error()})
		return
	}

	if PlayList == nil {
		PlayList = &PList{CurrentIndex: -1}
	}

	PlayList.AddEntry(entry)
	c.JSON(http.StatusCreated, entry)
}

func streamStatus(c *gin.Context) {
	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-StatusStream; ok {
			c.SSEvent("status", msg)
			return true
		}
		return false
	})
}

func broadcastStatus() {
	// syslogger.Info("Broadcast playing media...")
	// StatusStream <- PlayingMedia
}

func terminate(message string, code int) {
	fmt.Println(message)
	os.Exit(code)
}

func main() {
	LOG = newLogger()
	LOG.Printf("omx-remote-api v%v", version)

	// Check if player is installed
	if omxDetect() != nil {
		terminate("omxplayer is not installed", 1)
	}

	// Make sure nothing is running
	omxCleanup()

	// Start a remote command listener
	go omxListen()

	// Register as a zero config service
	LOG.Infof("Starting zeroconf service [%s]", zeroConfName)
	server, err := zeroconf.Register(zeroConfName, zeroConfService, zeroConfDomain, defaultPort, nil, nil)
	if err != nil {
		LOG.Errorf("Cannot start zeroconf service: %s", err.Error())
	}
	defer server.Shutdown()

	// Disable debugging mode
	gin.SetMode("release")

	// Setup HTTP server
	router := gin.New()
	router.Use(gin.Recovery())

	// CORS
	router.Use(cors.Default())

	// Logger
	router.Use(HTTPLogger(LOG))

	router.GET("/status", httpStatus)
	router.GET("/status/stream", streamStatus)
	router.POST("/play", httpPlay)
	router.POST("/commands/:command", httpCommand)

	// playlist management
	router.PUT("/plist", httpNewPList)
	router.POST("/plist/commands/next", httpPListNext)
	router.POST("/plist/commands/select", httpPListSelect)
	router.POST("/plist/entries/", httpPListAddEntry)

	LOG.Printf("Starting http server on 0.0.0.0:%d", defaultPort)
	router.Run(fmt.Sprintf(":%d", defaultPort))
}
