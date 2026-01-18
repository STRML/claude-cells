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

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user for Claude Code (--dangerously-skip-permissions requires non-root)
RUN useradd -m -s /bin/bash claude && \
    echo "claude ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers

# Set up directories with proper permissions
RUN mkdir -p /workspace /home/claude/.claude && \
    chown -R claude:claude /workspace /home/claude

USER claude
WORKDIR /workspace
CMD ["sleep", "infinity"]
