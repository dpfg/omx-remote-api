# omx-remote-api

The work is based on [sosedoff/omxremote](https://github.com/sosedoff/omxremote)

Notable changes:

* No html, only API
* Few fixes in API to make it more Restful
* Removed files discovery
* Added ability to play content through the URL
* Added ability to store unstructured info about playing media
* Log to syslog
* SSE Event to stream updates in plyaing media
* Playlist support

### API

#### Create new playlist

```json
//> PUT /plist
{
  "entries": [
    {
      "url": "http://media.mp4",
      "media_info": {
        "title": "Episode 1: Pilot"
      }
    },
    {
      "url": "http://media2.mp4",
      "media_info": {
        "title": "Episode 2"
      }
    }
  ]
}
```

#### Play next entry from the playlist

```json
//> POST /plist/commands/next
```

#### Play specific entry from the playlist (by its position)

```json
//> POST /plist/commands/select
{
  "position": 4
}
```

### Deployment to raspberry-pi

```bash
> make release pi-deploy
```
