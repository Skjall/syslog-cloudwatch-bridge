FROM golang as builder
WORKDIR /workspace
RUN go get -d -v github.com/aws/aws-sdk-go/aws \
	github.com/aws/aws-sdk-go/aws/session \
	github.com/aws/aws-sdk-go/service/cloudwatchlogs \
	github.com/satori/go.uuid 
RUN go get -d -v gopkg.in/davaops/go-syslog.v3
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o syslog-cloudwatch-bridge .

FROM scratch
EXPOSE 514
EXPOSE 514/udp
COPY --from=builder /workspace/syslog-cloudwatch-bridge /
CMD ["/syslog-cloudwatch-bridge"]
