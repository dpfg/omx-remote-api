package main

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

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
