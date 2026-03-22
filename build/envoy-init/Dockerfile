FROM alpine:3.19
RUN apk add --no-cache iptables
COPY init.sh /init.sh
RUN chmod +x /init.sh
ENTRYPOINT ["/init.sh"]
