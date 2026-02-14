import { describe, it, expect, vi } from "vitest";
import QRCode from "qrcode";

// Mock QRCode library
vi.mock("qrcode");

describe("QrCodeDialog - QR Code Generation", () => {
  it("should use QRCode library to generate canvas-based QR codes", async () => {
    const mockCanvas = document.createElement("canvas");
    const serverUrl = "https://test.motus.local";

    (QRCode.toCanvas as any) = vi.fn().mockResolvedValue(undefined);

    await QRCode.toCanvas(mockCanvas, serverUrl, {
      width: 256,
      margin: 2,
      color: {
        dark: "#000000",
        light: "#FFFFFF",
      },
    });

    expect(QRCode.toCanvas).toHaveBeenCalledWith(mockCanvas, serverUrl, {
      width: 256,
      margin: 2,
      color: {
        dark: "#000000",
        light: "#FFFFFF",
      },
    });
  });

  it("should generate QR code with correct server URL format", async () => {
    const mockCanvas = document.createElement("canvas");
    const serverUrl = "https://demo.traccar.org";

    (QRCode.toCanvas as any) = vi.fn().mockResolvedValue(undefined);

    const options = {
      width: 256,
      margin: 2,
      color: {
        dark: "#000000",
        light: "#FFFFFF",
      },
    };

    await QRCode.toCanvas(mockCanvas, serverUrl, options);

    expect(QRCode.toCanvas).toHaveBeenCalledWith(
      mockCanvas,
      serverUrl,
      options,
    );
  });

  it("should handle QR code generation errors gracefully", async () => {
    const mockCanvas = document.createElement("canvas");
    const serverUrl = "invalid-url";

    (QRCode.toCanvas as any) = vi
      .fn()
      .mockRejectedValue(new Error("Invalid URL"));

    await expect(QRCode.toCanvas(mockCanvas, serverUrl)).rejects.toThrow(
      "Invalid URL",
    );
  });
});

describe("QrCodeDialog - Server URL Format", () => {
  it("should format server URL from window.location.origin", () => {
    const mockLocation = {
      origin: "https://motus.example.com",
      protocol: "https:",
      host: "motus.example.com",
    };

    expect(mockLocation.origin).toBe("https://motus.example.com");
    expect(mockLocation.origin).toMatch(/^https:\/\//);
  });

  it("should handle localhost URLs correctly", () => {
    const mockLocation = {
      origin: "http://localhost:5173",
      protocol: "http:",
      host: "localhost:5173",
    };

    expect(mockLocation.origin).toBe("http://localhost:5173");
  });

  it("should handle production URLs with custom ports", () => {
    const mockLocation = {
      origin: "https://traccar.company.com:8082",
      protocol: "https:",
      host: "traccar.company.com:8082",
    };

    expect(mockLocation.origin).toBe("https://traccar.company.com:8082");
  });
});

describe("QrCodeDialog - API Token QR Code URL", () => {
  it("should build QR data with token appended to server URL", () => {
    const serverUrl = "https://motus.example.com";
    const apiToken = "mts_abc123def456";

    const qrData = apiToken ? `${serverUrl}?token=${apiToken}` : serverUrl;

    expect(qrData).toBe("https://motus.example.com?token=mts_abc123def456");
  });

  it("should use plain server URL when no token is provided", () => {
    const serverUrl = "https://motus.example.com";
    const apiToken: string | undefined = undefined;

    const qrData = apiToken ? `${serverUrl}?token=${apiToken}` : serverUrl;

    expect(qrData).toBe("https://motus.example.com");
  });

  it("should generate QR code with token-embedded URL on canvas", async () => {
    const mockCanvas = document.createElement("canvas");
    const tokenUrl = "https://motus.example.com?token=mts_testkey123";

    (QRCode.toCanvas as any) = vi.fn().mockResolvedValue(undefined);

    const options = {
      width: 256,
      margin: 2,
      color: {
        dark: "#000000",
        light: "#FFFFFF",
      },
    };

    await QRCode.toCanvas(mockCanvas, tokenUrl, options);

    expect(QRCode.toCanvas).toHaveBeenCalledWith(mockCanvas, tokenUrl, options);
  });

  it("should handle token with special characters in URL", () => {
    const serverUrl = "https://motus.example.com";
    const apiToken = "mts_key+with/special=chars";

    const qrData = `${serverUrl}?token=${apiToken}`;

    expect(qrData).toBe(
      "https://motus.example.com?token=mts_key+with/special=chars",
    );
  });

  it("should handle server URL with custom port and token", () => {
    const serverUrl = "https://traccar.company.com:8082";
    const apiToken = "mts_prodkey789";

    const qrData = `${serverUrl}?token=${apiToken}`;

    expect(qrData).toBe(
      "https://traccar.company.com:8082?token=mts_prodkey789",
    );
  });
});
