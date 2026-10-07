# The default agent sandbox: a general-purpose Linux machine. Agents run as root inside it and
# may install anything else they need (apt, pip, npm); /home/agent persists across restarts.
FROM debian:bookworm-slim

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bash ca-certificates curl wget git openssh-client \
      python3 python3-pip python3-venv nodejs npm \
      build-essential jq ripgrep less nano procps unzip zip file tree \
    && rm -rf /var/lib/apt/lists/*

ENV HOME=/home/agent
WORKDIR /home/agent
CMD ["sleep", "infinity"]
