FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && apt install -y \
    openssh-server bash jq ca-certificates netcat-openbsd && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/run/sshd && \
    useradd -m -s /bin/bash demo && \
    echo "demo:demo" | chpasswd && \
    sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config && \
    sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config && \
    sed -i 's/^#\?ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' /etc/ssh/sshd_config

COPY ./m1/lab_hosts/host02/data /opt/data
COPY ./m1/lab_hosts/host02/commands /opt/commands
COPY ./m1/lab_hosts/host02/scripts /opt/scripts

COPY ./m1/lab_hosts/common/agent.sh /opt/agent/agent.sh
COPY ./m1/lab_hosts/common/entrypoint.sh /opt/entrypoint.sh

RUN chmod +x /opt/commands/*.sh /opt/scripts/*.sh /opt/agent/agent.sh /opt/entrypoint.sh

EXPOSE 22

CMD ["/opt/entrypoint.sh"]
