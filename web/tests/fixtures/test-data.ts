export const TEST_CREDENTIALS = {
  email: 'admin@motus.local',
  password: 'admin',
};

export const INVALID_CREDENTIALS = {
  email: 'wrong@example.com',
  password: 'wrongpassword',
};

export const TEST_DEVICE = {
  name: 'PW Test Device',
  uniqueId: `pw-test-${Date.now()}`,
  phone: '+1234567890',
  model: 'TK103',
  category: 'car',
};

export const TEST_GEOFENCE = {
  name: 'PW Test Geofence',
  geometry: {
    type: 'Polygon' as const,
    coordinates: [[
      [11.5820, 48.1351],
      [11.5920, 48.1351],
      [11.5920, 48.1451],
      [11.5820, 48.1451],
      [11.5820, 48.1351],
    ]],
  },
};

export const TEST_NOTIFICATION = {
  name: 'PW Test Notification',
  eventTypes: ['geofenceEnter'],
  webhookUrl: 'https://webhook.site/test-webhook',
  template: '{"device": "{{device.name}}", "event": "{{event.type}}"}',
};

export const NAV_LINKS = [
  { label: 'Dashboard', path: '/' },
  { label: 'Devices', path: '/devices' },
  { label: 'Map', path: '/map' },
  { label: 'Reports', path: '/reports' },
  { label: 'Geofences', path: '/geofences' },
  { label: 'Notifications', path: '/notifications' },
];
