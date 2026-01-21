# Container Security Configuration

Claude Cells uses hardened container defaults to protect your system while running Claude Code instances. This document explains the security model, configuration options, and how to customize settings when needed.

## Overview

Containers run with security-hardened defaults that:
- Prevent privilege escalation attacks
- Block container escape vectors
- Limit resource consumption
- Maintain compatibility with most dev images

If a container fails to start due to security restrictions, Claude Cells automatically relaxes settings and saves the working configuration for future use.

## Security Tiers

Claude Cells uses three security tiers, from most to least restrictive:

### Hardened Tier
**Drops:** `SYS_ADMIN`, `SYS_MODULE`, `SYS_PTRACE`, `NET_ADMIN`, `NET_RAW`

Most secure tier. May break:
- `ping` and network diagnostic tools (NET_RAW)
- Debuggers using ptrace (gdb, strace)
- Some profilers

Use when: Maximum security is required and you don't need debugging tools.

### Moderate Tier (Default)
**Drops:** `SYS_ADMIN`, `SYS_MODULE`

Good balance of security and compatibility. Blocks:
- Container escape via `mount` (SYS_ADMIN)
- Kernel module loading (SYS_MODULE)

Works with most development images including debuggers.

### Compatible Tier
**Drops:** Nothing

Only applies `no-new-privileges` and init process. Use when:
- Other tiers break your specific dev image
- You're using specialized tools that require capabilities

## Configuration Files

Configuration is loaded from two locations, with project config taking precedence:

1. **Global:** `~/.claude-cells/config.yaml`
2. **Project:** `.claude-cells/config.yaml` (in project root)

### Example Configuration

```yaml
# ~/.claude-cells/config.yaml
security:
  # Security tier: "hardened", "moderate", or "compat"
  tier: moderate

  # Block setuid/setcap privilege escalation (recommended: true)
  no_new_privileges: true

  # Use init process for proper signal handling (recommended: true)
  init: true

  # Maximum processes in container (0 = unlimited)
  pids_limit: 1024

  # Capabilities to drop (overrides tier default if set)
  # cap_drop:
  #   - SYS_ADMIN
  #   - SYS_MODULE

  # Capabilities to add (use sparingly)
  # cap_add:
  #   - NET_RAW  # If you need ping

  # Auto-relax on container start failure (recommended: true)
  auto_relax: true

  # DANGEROUS OPTIONS - leave false unless you know what you're doing
  privileged: false      # Full host access
  host_network: false    # Host network namespace
  host_pid: false        # Host PID namespace
  host_ipc: false        # Host IPC namespace
  docker_socket: false   # Mount Docker socket
```

### Per-Project Override

To relax settings for a specific project, create `.claude-cells/config.yaml`:

```yaml
# .claude-cells/config.yaml
security:
  tier: compat           # Less restrictive for this project
  cap_add:
    - NET_RAW            # Enable ping for this project
```

## Auto-Relaxation

When `auto_relax: true` (default), Claude Cells automatically handles security-related startup failures:

1. **First attempt:** Uses configured tier (default: moderate)
2. **On failure:** Tries the next less restrictive tier
3. **On success:** Saves working config to `.claude-cells/config.yaml`
4. **Notification:** User is informed of the relaxation

The saved configuration ensures the same project won't fail on subsequent runs.

### Disabling Auto-Relaxation

```yaml
security:
  auto_relax: false
```

With auto-relax disabled, containers fail immediately if security settings are incompatible.

## What Each Setting Does

### `no_new_privileges`
Prevents processes from gaining new privileges via setuid/setcap binaries. This is always safe for development work and blocks a common privilege escalation vector.

### `init`
Runs an init process (tini) as PID 1. This ensures proper signal handling and prevents zombie processes. Always recommended.

### `pids_limit`
Limits the number of processes a container can create. Prevents fork bombs. Default of 1024 is generous for typical development.

### `cap_drop` / `cap_add`
Linux capabilities to drop or add. Dropping capabilities restricts what the container can do, even as root.

**Common capabilities:**
- `SYS_ADMIN` - Most dangerous. Required for `mount`, namespaces, many escape vectors
- `SYS_MODULE` - Loading kernel modules
- `SYS_PTRACE` - Process tracing (debuggers)
- `NET_ADMIN` - Network configuration
- `NET_RAW` - Raw sockets (ping, tcpdump)

### `privileged`
**DANGEROUS.** Grants full host access. Container can escape trivially. Only use for Docker-in-Docker or similar special cases.

### Host Namespaces (`host_network`, `host_pid`, `host_ipc`)
**DANGEROUS.** Shares host namespaces with container. Defeats isolation. Only enable if specifically required.

### `docker_socket`
**DANGEROUS.** Mounting the Docker socket allows the container to control Docker, effectively granting host-level access.

## Common Scenarios

### I need to use ping
```yaml
security:
  cap_add:
    - NET_RAW
```

### I need debuggers (gdb, strace)
```yaml
security:
  tier: moderate  # SYS_PTRACE is not dropped
```

### Container won't start at all
```yaml
security:
  tier: compat    # Most compatible
```

### I need Docker-in-Docker
```yaml
security:
  privileged: true  # Required for DinD
```

**Warning:** This defeats container isolation. Only use when absolutely necessary.

## Security Best Practices

1. **Start with defaults.** The moderate tier works for most development.

2. **Let auto-relax help.** If a container fails, auto-relax finds the minimum required relaxation.

3. **Be specific.** Instead of switching to `compat`, try adding just the capability you need.

4. **Review generated configs.** When auto-relax creates `.claude-cells/config.yaml`, review it to understand what was needed.

5. **Never enable `privileged` casually.** It should be a conscious, documented decision.

6. **Don't mount the Docker socket.** If you need Docker inside containers, consider alternatives like Sysbox or rootless Docker.

## Troubleshooting

### "operation not permitted" errors
The container is trying to do something blocked by dropped capabilities. Check logs to identify what capability is needed, then add it specifically.

### Debugger doesn't work
Ensure `SYS_PTRACE` is not dropped. The `moderate` tier preserves this capability.

### Network tools fail
Add `NET_RAW` capability for ping/tcpdump, or `NET_ADMIN` for network configuration.

### Container starts but behaves strangely
Check if `init` is enabled. Without init, signals may not be handled correctly.

## Reference

### Default Settings (Moderate Tier)
```yaml
security:
  tier: moderate
  no_new_privileges: true
  init: true
  pids_limit: 1024
  cap_drop:
    - SYS_ADMIN
    - SYS_MODULE
  cap_add: []
  privileged: false
  host_network: false
  host_pid: false
  host_ipc: false
  docker_socket: false
  auto_relax: true
```

### Capability Reference
| Capability | Purpose | Risk if Granted |
|------------|---------|-----------------|
| SYS_ADMIN | Mount, namespaces, many ops | Container escape |
| SYS_MODULE | Load kernel modules | Kernel compromise |
| SYS_PTRACE | Process tracing | Debug host processes |
| NET_ADMIN | Network configuration | Network manipulation |
| NET_RAW | Raw sockets | Network sniffing |
| DAC_OVERRIDE | Bypass file permissions | Read any file |
| SETUID/SETGID | Change user/group | Privilege escalation |

For a complete list, see `man 7 capabilities`.
