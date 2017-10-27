## Store

Simple in-memory key / value pair store implementation using Go.

Implements UDP / TCP / HTTP protocols.

### Usage

* Clone repository
* `cd main && go build store.go`
* `./store`

The output on the screen should be similar to: 
```
info - 2017-10-27T20:57:55Z - listening for http connections on: :18082
info - 2017-10-27T20:57:55Z - listening for udp connections on: :18080
info - 2017-10-27T20:57:55Z - listening for tcp connections on: :18081
``` 

This displays the ports the server is listening on.

The interface is as follows:

##### UDP

UDP requests require input in the form of a simple JSON object in the form:
```
{"operation": "get", "key": "<key_to_set>", "data": "<data_to_set>"}
```

the data field is only required when the operation is `set`.

eg: `echo -n '{"operation": "get", "key": "a-store-key"}' | nc -4u -w1 localhost:18080`

##### TCP

TCP takes the exact same JSON document as a UDP request. 

eg: `echo -n '{"operation": "get", "key": "a-store-key"}' | nc localhost:18081`

##### HTTP

HTTP has a more structured interface, in which we have one single endpoint `/store/{key}` and a GET request retrieves data
and a POST request pushes data into the store.

eg:
```
curl -X POST -d 'some-data' http://localhost:18082/store/a-store-key //-> returns {"status_code": 0, "key": "a-store-key"}`
curl -X GET http://localhost:18082/store/a-store-key //-> returns {"status_code": 0, "key": "a-store-key", "data": "some-data"}`
``` 

There is also a endpoint which retrieves a count of the amount of items
currently in the store.
This is `/api/count`.
