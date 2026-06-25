// ==================== Shared Constants ====================

export const CHAIN_OPTIONS = ['INPUT', 'OUTPUT', 'FORWARD'];
export const PROTOCOL_OPTIONS = ['tcp', 'udp', 'all', 'icmp'];
export const ACTION_OPTIONS = ['ACCEPT', 'DROP', 'REJECT'];
export const IP_VERSION_OPTIONS = [
  { label: 'IPv4', value: 'ipv4' },
  { label: 'IPv6', value: 'ipv6' },
  { label: '双栈', value: 'both' },
];

export const disabledRowStyle = `
.firewall-rule-disabled {
  opacity: 0.5;
  background-color: #f5f5f5;
}
`;

export const actionColor = (action: string) => {
  switch (action) {
    case 'ACCEPT': return 'success';
    case 'DROP': return 'error';
    case 'REJECT': return 'warning';
    default: return 'default';
  }
};
