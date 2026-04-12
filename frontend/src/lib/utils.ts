import { emptyTailscale, routeRank } from './constants';

export function normalizeTailscale(status) {
  return {
    ...emptyTailscale,
    ...(status || {}),
    selfIPs: status?.selfIPs || [],
  };
}

export function shortId(id) {
  return id ? id.slice(0, 12) : 'unassigned';
}

export function shortFingerprint(fingerprint) {
  if (!fingerprint) {
    return 'pending';
  }
  const raw = fingerprint.slice(0, 24);
  return raw.match(/.{1,4}/g)?.join(':') || raw;
}

export function timeAgo(timestamp) {
  if (!timestamp) {
    return 'never';
  }
  const elapsed = Math.max(0, Math.floor(Date.now() / 1000 - timestamp));
  if (elapsed < 5) {
    return 'just now';
  }
  if (elapsed < 60) {
    return `${elapsed}s ago`;
  }
  if (elapsed < 3600) {
    return `${Math.floor(elapsed / 60)}m ago`;
  }
  if (elapsed < 86400) {
    return `${Math.floor(elapsed / 3600)}h ago`;
  }
  return `${Math.floor(elapsed / 86400)}d ago`;
}

export function orderedRoutes(routes = []) {
  return [...routes].sort((left, right) => {
    const leftRank = routeRank[left] ?? 9;
    const rightRank = routeRank[right] ?? 9;
    if (leftRank === rightRank) {
      return left.localeCompare(right);
    }
    return leftRank - rightRank;
  });
}

export function routeLabel(route) {
  if (route === 'lan') {
    return 'LAN';
  }
  if (route === 'tailscale') {
    return 'Tailnet';
  }
  if (route === 'manual') {
    return 'Manual';
  }
  return route;
}

export function preferredRouteLabel(route) {
  if (!route) {
    return 'Auto';
  }
  return routeLabel(route);
}

export function sessionStateLabel(session) {
  if (!session.connected) {
    return 'Idle';
  }
  if (session.controlling) {
    return 'Controlling';
  }
  if (session.role === 'controlled') {
    return 'Controlled';
  }
  return 'Connected';
}

export function meshStateLabel(tailscale) {
  if (!tailscale.available) {
    return 'Unavailable';
  }
  if (tailscale.connected) {
    return 'Connected';
  }
  return tailscale.backendState || 'Degraded';
}

export function peerAddresses(peer) {
  return (peer.addresses || []).join(', ');
}

export function trustLabel(peer) {
  return peer?.trusted ? 'Pinned' : 'PIN required';
}

export async function copyToClipboard(value) {
  if (!value || !navigator?.clipboard?.writeText) {
    return;
  }
  await navigator.clipboard.writeText(value);
}

export function formatLatency(ms) {
  if (ms == null || ms < 0) {
    return 'Measuring...';
  }
  if (ms === 0) {
    return '<1 ms';
  }
  return `${ms} ms`;
}

// Returns 'good' | 'ok' | 'warn' | 'bad' | 'measuring' based on thresholds [good, ok, warn].
export function latencyQuality(ms, thresholds) {
  if (ms == null || ms < 0) return 'measuring';
  if (ms <= thresholds[0]) return 'good';
  if (ms <= thresholds[1]) return 'ok';
  if (ms <= thresholds[2]) return 'warn';
  return 'bad';
}

export function healthSummary(health) {
  if (!health) {
    return 'Unknown';
  }
  if (health.reconnecting) {
    return 'Reconnecting...';
  }
  const unhealthy = (health.subsystems || []).filter((s) => !s.healthy);
  if (unhealthy.length > 0) {
    return `Degraded: ${unhealthy.map((s) => s.name).join(', ')}`;
  }
  return 'Healthy';
}
