ARG BASE_IMAGE=ubuntu:22.04
FROM ${BASE_IMAGE}

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    ca-certificates \
    xz-utils \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js LTS (architecture-aware)
ARG TARGETARCH
RUN ARCH=$(case "${TARGETARCH}" in "arm64") echo "arm64" ;; *) echo "x64" ;; esac) && \
    curl -fsSL https://nodejs.org/dist/v20.11.0/node-v20.11.0-linux-${ARCH}.tar.xz \
    | tar -xJ -C /usr/local --strip-components=1

# Install Claude Code CLI (native installer)
RUN curl -fsSL https://claude.ai/install.sh | bash

# Install claude-sneakpeek (experimental build with swarm mode)
RUN npx @realmikekelly/claude-sneakpeek quick --name claudesp

# Install GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# Set up directories (running as root with IS_SANDBOX=1)
RUN mkdir -p /workspace /root/.claude

WORKDIR /workspace
CMD ["sleep", "infinity"]
