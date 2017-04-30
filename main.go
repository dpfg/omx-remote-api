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

	"github.com/gin-gonic/gin"
)

const version = "0.0.2"

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

	// CurrentURL represents currently played media
	CurrentURL *url.URL

	// Syslog logger
	syslogger *syslog.Writer
)

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
			Omx.Process.Kill()
		}
	}
}

// Start omxplayer playback for a given video file. Returns error if start fails.
func omxPlay(url *url.URL) error {
	Omx = exec.Command(
		OmxPath,      // path to omxplayer executable
		"--blank",    // set background to black
		"--adev",     // audio out device
		"hdmi",       // using hdmi for audio/video
		url.String(), // path to video file
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
	CurrentURL = url

	// Make child's STDIN globally available
	OmxIn = stdin

	// Wait until child process is finished
	err = Omx.Wait()
	if err != nil {
		syslogger.Err(fmt.Sprintln("Process exited with error:", err))
	}

	omxCleanup()

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
	CurrentURL = nil

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

	loc, err := url.Parse(c.Query("url"))
	if err != nil {
		c.JSON(http.StatusBadRequest, APIErr{"Invalid content location"})
		return
	}

	go omxPlay(loc)

	c.Status(http.StatusAccepted)
}

func httpStatus(c *gin.Context) {
	result := struct {
		Running    bool   `json:"running,omitempty"`
		CurrentURL string `json:"currentURL,omitempty"`
	}{
		Running:    omxIsActive(),
		CurrentURL: toString(CurrentURL),
	}

	c.JSON(http.StatusOK, result)
}

func toString(url *url.URL) string {
	if url == nil {
		return ""
	}

	return url.String()
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

	router.GET("/status", httpStatus)
	router.POST("/play", httpPlay)
	router.POST("/commands/:command", httpCommand)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Starting server on 0.0.0.0:" + port)
	router.Run(":" + port)
}
