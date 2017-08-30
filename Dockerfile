FROM alpine:3.6
RUN apk add --no-cache ca-certificates
COPY gopath/bin/cloud-dyndns-client /cloud-dyndns-client
EXPOSE 8080
CMD ["/cloud-dyndns-client"]
