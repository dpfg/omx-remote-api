# omx-remote-api

The work is based on [sosedoff/omxremote](https://github.com/sosedoff/omxremote)

Notable changes:

- No html, only API
- Few fixes in API to make it more Restful
- Removed files discovery
- Added ability to play content through the URL
- Added ability to store unstructured info about playing media
- Log to syslog
- SSE Event to stream updates in plyaing media


### Deployment to raspberry-pi
```
> make release pi-deploy
```