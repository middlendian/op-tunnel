FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        openssh-server \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd

# sshd configuration
RUN echo 'AcceptEnv LC_OP_TUNNEL_SOCK' >> /etc/ssh/sshd_config \
    && echo 'StreamLocalBindUnlink yes' >> /etc/ssh/sshd_config \
    && echo 'PermitRootLogin yes' >> /etc/ssh/sshd_config

# Install op-tunnel-client, symlinked as `op`
COPY op-tunnel-client /usr/local/bin/op-tunnel-client
RUN chmod +x /usr/local/bin/op-tunnel-client \
    && ln -s /usr/local/bin/op-tunnel-client /usr/local/bin/op

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22
CMD ["/entrypoint.sh"]
