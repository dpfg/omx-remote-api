package main

import (
	"fmt"
	"io"
	"log/syslog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	cors "gopkg.in/gin-contrib/cors.v1"

	"github.com/gin-gonic/gin"
)

const version = "0.0.4"

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

	// Command is a channel to pass along commands to the player routine
	Command chan string

	// StatusStream channel to broadcast any changes in playing media via SSE
	StatusStream = make(chan *MediaEntry)

	// PlayingMedia represents currently playing media
	PlayingMedia *MediaEntry

	// Syslog logger
	syslogger *syslog.Writer
)

// MediaEntry describes model of currently playable video.
type MediaEntry struct {
	RawURL    string                 `json:"url,omitempty"`
	MediaInfo map[string]interface{} `json:"mediaInfo,omitempty"`
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
			err := Omx.Process.Kill()
			if err != nil {
				syslogger.Err(err.Error())
			}
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

	// Redirect output for debugging purposes
	// Omx.Stdout = os.Stdout

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

	// Wait until child process is finished
	err = Omx.Wait()
	if err != nil {
		syslogger.Err(fmt.Sprintln("Process exited with error:", err))
	}

	omxCleanup()

	broadcastStatus()

	return nil
}

// Write a command string to the omxplayer process's STDIN
func omxWrite(command string) {
	if OmxIn != nil {
		io.WriteString(OmxIn, Commands[command])
	}
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
	}{
		Running:    omxIsActive(),
		MediaEntry: PlayingMedia,
	}

	c.JSON(http.StatusOK, result)
}

func streamStatus(c *gin.Context) {
	c.Stream(func(w io.Writer) bool {

		var data interface{}
		if entry := <-StatusStream; entry != nil {
			data = entry
		} else {
			data = struct{}{}
		}

		c.SSEvent("status", data)
		return true
	})
}

func broadcastStatus() {
	syslogger.Info("Broadcast playing media...")
	StatusStream <- PlayingMedia
}

func terminate(message string, code int) {
	fmt.Println(message)
	os.Exit(code)
}

func main() {
	fmt.Printf("omx-remote-api v%v\n", version)

	syslogger, _ = syslog.New(syslog.LOG_NOTICE, "omx-remote-api")

	// Check if player is installed
	if omxDetect() != nil {
		terminate("omxplayer is not installed", 1)
	}

	// Make sure nothing is running
	omxCleanup()

	// Start a remote command listener
	go omxListen()

	// Disable debugging mode
	gin.SetMode("release")
	gin.LoggerWithWriter(syslogger)

	// Setup HTTP server
	router := gin.Default()

	// CORS
	router.Use(cors.Default())

	router.GET("/status", httpStatus)
	router.GET("/status/stream", streamStatus)
	router.POST("/play", httpPlay)
	router.POST("/commands/:command", httpCommand)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Starting server on 0.0.0.0:" + port)
	router.Run(":" + port)
}
