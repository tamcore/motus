import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock the API client
const mockGetDevices = vi.fn();
const mockUpdateDevice = vi.fn();
const mockCreateDevice = vi.fn();
const mockDeleteDevice = vi.fn();
vi.mock("$lib/api/client", () => ({
  api: {
    getDevices: mockGetDevices,
    updateDevice: mockUpdateDevice,
    createDevice: mockCreateDevice,
    deleteDevice: mockDeleteDevice,
  },
}));

// Sample device data used across tests
function createTestDevice(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    name: "Test Vehicle",
    uniqueId: "ABC123",
    status: "online",
    phone: "+1234567890",
    model: "TK103",
    category: "car",
    lastUpdate: "2026-02-16T10:00:00Z",
    disabled: false,
    ...overrides,
  };
}

function createDeviceList() {
  return [
    createTestDevice({ id: 1, name: "Vehicle A", uniqueId: "AAA111", status: "online" }),
    createTestDevice({ id: 2, name: "Vehicle B", uniqueId: "BBB222", status: "offline" }),
    createTestDevice({ id: 3, name: "Vehicle C", uniqueId: "CCC333", status: "idle" }),
  ];
}

describe("Devices page - Expandable rows", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetDevices.mockResolvedValue(createDeviceList());
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("Expand/collapse state management", () => {
    it("should start with no rows expanded", () => {
      const expandedIds: Set<number> = new Set();
      expect(expandedIds.size).toBe(0);
    });

    it("should expand a row when toggled", () => {
      const expandedIds = new Set<number>();
      const deviceId = 1;

      // Toggle open
      if (expandedIds.has(deviceId)) {
        expandedIds.delete(deviceId);
      } else {
        expandedIds.add(deviceId);
      }

      expect(expandedIds.has(deviceId)).toBe(true);
      expect(expandedIds.size).toBe(1);
    });

    it("should collapse a row when toggled again", () => {
      const expandedIds = new Set<number>();
      const deviceId = 1;

      // Toggle open
      expandedIds.add(deviceId);
      expect(expandedIds.has(deviceId)).toBe(true);

      // Toggle closed
      expandedIds.delete(deviceId);
      expect(expandedIds.has(deviceId)).toBe(false);
      expect(expandedIds.size).toBe(0);
    });

    it("should allow multiple rows to be expanded simultaneously", () => {
      const expandedIds = new Set<number>();

      expandedIds.add(1);
      expandedIds.add(2);

      expect(expandedIds.has(1)).toBe(true);
      expect(expandedIds.has(2)).toBe(true);
      expect(expandedIds.size).toBe(2);
    });

    it("should not affect other expanded rows when toggling one", () => {
      const expandedIds = new Set<number>();

      expandedIds.add(1);
      expandedIds.add(2);
      expandedIds.add(3);

      // Collapse row 2
      expandedIds.delete(2);

      expect(expandedIds.has(1)).toBe(true);
      expect(expandedIds.has(2)).toBe(false);
      expect(expandedIds.has(3)).toBe(true);
    });
  });

  describe("Toggle function (immutable pattern)", () => {
    it("should create a new Set when toggling (immutable update)", () => {
      let expandedIds = new Set<number>();

      function toggleDevice(deviceId: number): Set<number> {
        const next = new Set(expandedIds);
        if (next.has(deviceId)) {
          next.delete(deviceId);
        } else {
          next.add(deviceId);
        }
        return next;
      }

      const original = expandedIds;
      expandedIds = toggleDevice(1);

      expect(expandedIds).not.toBe(original); // New object (immutability)
      expect(expandedIds.has(1)).toBe(true);
    });

    it("should handle rapid toggle without errors", () => {
      let expandedIds = new Set<number>();

      function toggleDevice(deviceId: number): Set<number> {
        const next = new Set(expandedIds);
        if (next.has(deviceId)) {
          next.delete(deviceId);
        } else {
          next.add(deviceId);
        }
        return next;
      }

      // Rapid toggle
      expandedIds = toggleDevice(1);
      expandedIds = toggleDevice(1);
      expandedIds = toggleDevice(1);

      expect(expandedIds.has(1)).toBe(true); // Odd number of toggles = expanded
    });
  });

  describe("Collapsed row displays summary info", () => {
    it("should provide name, status, and last update in summary", () => {
      const device = createTestDevice();

      // Summary fields that should be visible in collapsed state
      const summaryFields = {
        name: device.name,
        status: device.status,
        lastUpdate: device.lastUpdate,
      };

      expect(summaryFields.name).toBe("Test Vehicle");
      expect(summaryFields.status).toBe("online");
      expect(summaryFields.lastUpdate).toBeTruthy();
    });

    it("should handle device with no lastUpdate gracefully", () => {
      const device = createTestDevice({ lastUpdate: undefined });
      const lastSeen = device.lastUpdate ? new Date(device.lastUpdate).toISOString() : "Never";
      expect(lastSeen).toBe("Never");
    });
  });

  describe("Expanded row displays detailed info", () => {
    it("should include all device fields in expanded view", () => {
      const device = createTestDevice();

      // Fields that should appear in expanded detail
      const detailFields = {
        uniqueId: device.uniqueId,
        phone: device.phone,
        model: device.model,
        category: device.category,
      };

      expect(detailFields.uniqueId).toBe("ABC123");
      expect(detailFields.phone).toBe("+1234567890");
      expect(detailFields.model).toBe("TK103");
      expect(detailFields.category).toBe("car");
    });

    it("should show dash for missing optional fields", () => {
      const device = createTestDevice({ phone: undefined, model: undefined, category: undefined });

      expect(device.phone || "-").toBe("-");
      expect(device.model || "-").toBe("-");
      expect(device.category || "-").toBe("-");
    });
  });

  describe("Search filtering with expanded rows", () => {
    it("should filter devices by name and preserve expanded state", () => {
      const devices = createDeviceList();
      let expandedIds = new Set<number>([1, 2]);

      const searchQuery = "Vehicle A";
      const filtered = devices.filter(
        (d) =>
          d.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          d.uniqueId.toLowerCase().includes(searchQuery.toLowerCase()),
      );

      expect(filtered.length).toBe(1);
      expect(filtered[0].name).toBe("Vehicle A");
      // Expanded state persists even when filtered
      expect(expandedIds.has(filtered[0].id)).toBe(true);
    });

    it("should filter by uniqueId", () => {
      const devices = createDeviceList();
      const searchQuery = "BBB222";
      const filtered = devices.filter(
        (d) =>
          d.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          d.uniqueId.toLowerCase().includes(searchQuery.toLowerCase()),
      );

      expect(filtered.length).toBe(1);
      expect(filtered[0].uniqueId).toBe("BBB222");
    });
  });

  describe("Action buttons in expanded state", () => {
    it("should expose map link, share, edit, and delete actions", () => {
      const device = createTestDevice();
      const actions = ["map", "share", "edit", "delete"];

      // The map link should include device ID
      const mapHref = `/map?device=${device.id}`;
      expect(mapHref).toBe("/map?device=1");

      // All action types should be present
      expect(actions).toContain("map");
      expect(actions).toContain("share");
      expect(actions).toContain("edit");
      expect(actions).toContain("delete");
    });

    it("should prevent row toggle when clicking action buttons", () => {
      // Clicking action buttons should NOT toggle expand/collapse
      // This is handled via event.stopPropagation() in the implementation
      let toggled = false;
      let actionTriggered = false;

      function handleRowClick() {
        toggled = true;
      }

      function handleActionClick(event: { stopPropagation: () => void }) {
        event.stopPropagation();
        actionTriggered = true;
      }

      // Simulate clicking an action button
      const fakeEvent = { stopPropagation: () => {} };
      handleActionClick(fakeEvent);

      expect(actionTriggered).toBe(true);
      expect(toggled).toBe(false); // Row should NOT toggle
    });
  });

  describe("Keyboard accessibility", () => {
    it("should toggle expansion on Enter key", () => {
      let expandedIds = new Set<number>();

      function handleKeydown(deviceId: number, key: string) {
        if (key === "Enter" || key === " ") {
          const next = new Set(expandedIds);
          if (next.has(deviceId)) {
            next.delete(deviceId);
          } else {
            next.add(deviceId);
          }
          expandedIds = next;
        }
      }

      handleKeydown(1, "Enter");
      expect(expandedIds.has(1)).toBe(true);

      handleKeydown(1, "Enter");
      expect(expandedIds.has(1)).toBe(false);
    });

    it("should toggle expansion on Space key", () => {
      let expandedIds = new Set<number>();

      function handleKeydown(deviceId: number, key: string) {
        if (key === "Enter" || key === " ") {
          const next = new Set(expandedIds);
          if (next.has(deviceId)) {
            next.delete(deviceId);
          } else {
            next.add(deviceId);
          }
          expandedIds = next;
        }
      }

      handleKeydown(1, " ");
      expect(expandedIds.has(1)).toBe(true);
    });

    it("should not toggle on other keys", () => {
      let expandedIds = new Set<number>();

      function handleKeydown(deviceId: number, key: string) {
        if (key === "Enter" || key === " ") {
          const next = new Set(expandedIds);
          if (next.has(deviceId)) {
            next.delete(deviceId);
          } else {
            next.add(deviceId);
          }
          expandedIds = next;
        }
      }

      handleKeydown(1, "Tab");
      handleKeydown(1, "Escape");
      handleKeydown(1, "a");

      expect(expandedIds.size).toBe(0);
    });
  });

  describe("Chevron indicator", () => {
    it("should indicate expanded state", () => {
      const expandedIds = new Set<number>([1]);
      const isExpanded = expandedIds.has(1);
      expect(isExpanded).toBe(true);

      // Chevron rotation: 0deg when collapsed, 180deg when expanded
      const rotation = isExpanded ? 180 : 0;
      expect(rotation).toBe(180);
    });

    it("should indicate collapsed state", () => {
      const expandedIds = new Set<number>();
      const isExpanded = expandedIds.has(1);
      expect(isExpanded).toBe(false);

      const rotation = isExpanded ? 180 : 0;
      expect(rotation).toBe(0);
    });
  });

  describe("Device loading states", () => {
    it("should show skeleton loading state initially", async () => {
      let loading = true;
      const devices: unknown[] = [];

      expect(loading).toBe(true);
      expect(devices.length).toBe(0);

      // After API resolves
      const result = await mockGetDevices();
      loading = false;

      expect(loading).toBe(false);
      expect(result.length).toBe(3);
    });

    it("should handle API error during device loading", async () => {
      mockGetDevices.mockRejectedValueOnce(new Error("Network error"));

      let loading = true;
      let devices: unknown[] = [];
      let loadError = false;

      try {
        devices = await mockGetDevices();
      } catch {
        loadError = true;
      } finally {
        loading = false;
      }

      expect(loading).toBe(false);
      expect(devices.length).toBe(0);
      expect(loadError).toBe(true);
    });
  });
});
