echo build windows
SET CGO_ENABLED=0
SET GOOS=windows
SET GOARCH=amd64
go build main.go
echo build linux
SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64
go build main.go