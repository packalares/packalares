FROM alpine:3.19
RUN apk add --no-cache samba samba-common-tools samba-server

COPY smb.conf /etc/samba/smb.conf
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

RUN mkdir -p /packalares/data

EXPOSE 445 139

ENTRYPOINT ["/entrypoint.sh"]
