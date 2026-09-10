FROM docker:27-cli

RUN apk add --no-cache iptables iptables-legacy
COPY workload-firewall /usr/local/bin/workload-firewall
RUN chmod 0755 /usr/local/bin/workload-firewall

ENTRYPOINT ["/usr/local/bin/workload-firewall"]
CMD ["run"]
