package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	cors "github.com/gin-contrib/cors"

	"github.com/gin-gonic/gin"
	"github.com/grandcat/zeroconf"
	"github.com/sirupsen/logrus"
)

const (
	version     = "0.0.5b3"
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
		"next_audio_stream": "k",            // next audio stream
		"prev_audio_stream": "j",            // previous audio stream
	}

	// OmxPath is path to omxplayer executable
	OmxPath string

	// Omx is a child process for spawning omxplayer
	Omx *exec.Cmd

	// OmxIn is a child process STDIN pipe to send commands
	OmxIn io.WriteCloser

	// Command is a channel to pass along commands to the player routine
	Command chan string

	// StatusStream channel to broadcast any changes in playing media via SSE
	StatusStream = make(chan *MediaEntry)

	// PlayingMedia represents currently playing media
	PlayingMedia *MediaEntry

	// PlayList is a list of media entries to play sequentially
	PlayList = NewPlayList(make([]MediaEntry, 0))

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
	}
}

// Start omxplayer playback for a given video file. Returns error if start fails.
func omxPlay(c MediaEntry) error {
	// reset autoplay flag
	// PlayList.AutoPlay = true

	contentURL, err := url.Parse(c.RawURL)
	if err != nil {
		return err
	}

	Omx = exec.Command(
		OmxPath,   // path to omxplayer executable
		"--blank", // set background to black
		// "--stats",           // Pts and buffer stats
		// "--with-info",       // dump stream format before playback
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

	// Grab child process STDERR
	stderr, err := Omx.StderrPipe()
	if err != nil {
		return err
	}

	defer stderr.Close()

	// read child process STDOUT to get status
	// status := OmxProcessStatus{Stdout: stdout, Stderr: stderr, Logger: LOG}
	// status.Start()

	// Start omxplayer execution.
	// If successful, something will appear on HDMI display.
	err = Omx.Start()
	if err != nil {
		return err
	}

	setPlayingMedia(&c)

	// Make child's STDIN globally available
	OmxIn = stdin

	// Wait until child process is finished
	err = Omx.Wait()
	if err != nil {
		LOG.Error(fmt.Sprintln("Process exited with error:", err))
	} else {
		LOG.Info("Process exited without errors.")
	}

	omxCleanup()

	// if PlayList.AutoPlay {

	// 	if next := PlayList.Next(); next != nil {
	// 		go omxPlay(*next)
	// 	}
	// }

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

	PlayList.AutoPlay = false
	PlayList.CurrentIndex = positionNone

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
	setPlayingMedia(nil)

	omxKill()
}

// Check if player is currently active
func omxIsActive() bool {
	return Omx != nil
}

func setPlayingMedia(m *MediaEntry) {
	LOG.WithField("prefix", "state").Debug("new state: %+v", m)

	PlayingMedia = m

	select {
	case StatusStream <- m:
		LOG.WithField("prefix", "broadcaster").Debug("sent update: %+v", m)
	default:
	}
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
	router.POST("/plist/entries", httpPListAddEntry)
	router.DELETE("/plist", httpPlistDelete)

	LOG.Printf("Starting http server on 0.0.0.0:%d", defaultPort)
	router.Run(fmt.Sprintf(":%d", defaultPort))
}
