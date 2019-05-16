package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
    "bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/satori/go.uuid"
    
	"gopkg.in/davaops/go-syslog.v3"
    "gopkg.in/davaops/go-syslog.v3/format"
)

var port = os.Getenv("PORT")
var logGroupName = os.Getenv("LOG_GROUP_NAME")
var streamName = os.Getenv("LOG_STREAM_NAME")
    
var sequenceToken = ""

var (
	client *http.Client
	pool   *x509.CertPool
)

func init() {
	pool = x509.NewCertPool()
	pool.AppendCertsFromPEM(pemCerts)
}

func main() {
	if logGroupName == "" {
		log.Fatal("LOG_GROUP_NAME must be specified")
	}
	if streamName == "" {
		var tempStreamName, err = uuid.NewV4()
		streamName = tempStreamName
	}

	if port == "" {
		port = "514"
	}

	address := fmt.Sprintf("0.0.0.0:%v", port)
	log.Println("Starting syslog server on", address)
	log.Println("Logging to group:", logGroupName)
	initCloudWatchStream()

	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.Automatic)
	server.SetHandler(handler)
	server.ListenUDP(address)
	server.ListenTCP(address)

	server.Boot()

	go func(channel syslog.LogPartsChannel) {
		for logParts := range channel {
			sendToCloudWatch(logParts)
		}
	}(channel)

	server.Wait()
}

func sendToCloudWatch(logPart format.LogParts) {
	// service is defined at run time to avoid session expiry in long running processes
	var svc = cloudwatchlogs.New(session.New())
	// set the AWS SDK to use our bundled certs for the minimal container (certs from CoreOS linux)
	svc.Config.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	params := &cloudwatchlogs.PutLogEventsInput{
		LogEvents: []*cloudwatchlogs.InputLogEvent{
			{
				Message:   aws.String(formatMessageContent(logPart)),
				Timestamp: aws.Int64(makeMilliTimestamp(logPart["timestamp"].(time.Time))),
			},
		},
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(streamName.String()),
	}

	// first request has no SequenceToken - in all subsequent request we set it
	if sequenceToken != "" {
		params.SequenceToken = aws.String(sequenceToken)
	}

	resp, err := svc.PutLogEvents(params)
	if err != nil {
		log.Println(err)
	}

	sequenceToken = *resp.NextSequenceToken
}

func initCloudWatchStream() {
	// service is defined at run time to avoid session expiry in long running processes
	var svc = cloudwatchlogs.New(session.New())
	// set the AWS SDK to use our bundled certs for the minimal container (certs from CoreOS linux)
	svc.Config.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	_, err := svc.CreateLogStream(&cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(streamName.String()),
	})

	if err != nil {
		log.Fatal(err)
	}

	log.Println("Created CloudWatch Logs stream:", streamName)
}


func makeMilliTimestamp(input time.Time) int64 {
	return input.UnixNano() / int64(time.Millisecond)
}

//Receives the logParts map and returns the string message in format <hostname> <tag/app_name> [<proc_id>]: <content>
func formatMessageContent(message format.LogParts) string {
    var buffer bytes.Buffer
    if message["hostname"] != nil &&  message["hostname"] != " " {
        buffer.WriteString(message["hostname"].(string))
        buffer.WriteString(" ")
    }
    if message["app_name"] != nil && message["app_name"] != " " {
        buffer.WriteString(message["app_name"].(string))
    } else {
        buffer.WriteString("-")
    }
    if message["proc_id"] != nil && message["proc_id"] != " " && message["proc_id"] != "-" {
        buffer.WriteString("[")
        buffer.WriteString(message["proc_id"].(string))
        buffer.WriteString("]")
    } else if message["pid"] != nil && message["pid"] != " " && message["pid"] != "-" {
        buffer.WriteString("[")
        buffer.WriteString(message["pid"].(string))
        buffer.WriteString("]")
    }
    buffer.WriteString(": ")
    if message["message"] != nil && message["message"] != " " { 
        buffer.WriteString(message["message"].(string))
    }   
    return buffer.String()
}
