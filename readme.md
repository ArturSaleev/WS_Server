# WS_Server

WebSocket server for project "Proctoring.kz"

### Create exe file for Windows
- GOOS=windows GOARCH=amd64 go build -o WSWindows.exe

### Create file for Linux
- GOOS=linux GOARCH=amd64 go build -o WSLinux

### Create file for MacOS
- GOOS=darwin GOARCH=amd64 go build -o WSMac

### Connected URL
- ws://127.0.0.1:8080/ws?user_id=1

### Connected URL To room
- ws://127.0.0.1:8080/ws?user_id=1&room_id=1

### SendMessage via WebSocket body
```
{
  "type": "message",
  "message": "hello world",
  "room": "1",
  "user_ids": ["2"]
}
```


### SendMessage for POST Request
- curl --location 'http://127.0.0.1:8080/send' \
  --header 'Content-Type: application/json' \
  --data '{
  "type" : "message",
  "message": "Hello world",
  "body": "Hello world big text",
  "user_ids": ["1", "2", "3", "4"]
  }' 

