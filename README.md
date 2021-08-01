# marine-chain-tech-test

Run

```
$ go get
$ go test
```

or manually

use help

```
$ go run main.go -help
```

run public instence

```
$ go run main.com -mode public -listen 127.0.0.1:8000
```

run private "servers"

```
$ go run main.com -mode private -id 0000 -listen 127.0.0.1:8100 -public http://127.0.0.1:8000
$ go run main.com -mode private -id 0001 -listen 127.0.0.1:8101 -public http://127.0.0.1:8000
$ go run main.com -mode private -id 0002 -listen 127.0.0.1:8102 -public http://127.0.0.1:8000
$ go run main.com -mode private -id 0003 -listen 127.0.0.1:8103 -public http://127.0.0.1:8000
$ go run main.com -mode private -id 0004 -listen 127.0.0.1:8104 -public http://127.0.0.1:8000
```

send file 

```
$ curl -XPUT "http://127.0.0.1:8000?filename=test" -d '1234567'
```

read  file

```
$ curl -XGET "http://127.0.0.1:8000?filename=test"
1234567
```
