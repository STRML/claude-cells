ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/base:ubuntu
FROM ${BASE_IMAGE}

# Install Node.js LTS
RUN curl -fsSL https://nodejs.org/dist/v20.11.0/node-v20.11.0-linux-x64.tar.xz \
    | tar -xJ -C /usr/local --strip-components=1

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code

WORKDIR /workspace
CMD ["sleep", "infinity"]
