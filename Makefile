release:
	GOOS=linux GOARCH=arm GOARM=7 go build

pi-deploy:
	ssh pi@raspberrypi.local 'mkdir -p /home/pi/Projects/omx-remote-api'
	scp omx-remote-api pi@raspberrypi.local:/home/pi/Projects/omx-remote-api
	ssh pi@raspberrypi.local 'sudo service omx-remote-api stop'
	ssh pi@raspberrypi.local 'sudo cp /home/pi/Projects/omx-remote-api/omx-remote-api /usr/bin/'
	ssh pi@raspberrypi.local 'sudo service omx-remote-api start'
