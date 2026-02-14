# QR Code Login Feature

## Overview

The QR Code Login feature allows Traccar Manager app users to quickly configure their server connection by scanning a QR code displayed on the Motus web interface login screen. This feature maintains 100% compatibility with the official Traccar mobile apps.

## Compatibility

### Traccar Manager (iOS/Android)
- **Status**: ✅ Fully Compatible
- **How it works**: Traccar Manager is a WebView-based application that displays the Motus web interface. The QR code button automatically appears when viewing the login page in the app.
- **User Experience**: Users can tap the QR code icon, scan the displayed code, and the app will automatically configure the server URL.

### Traccar Client (GPS Tracking App)
- **Status**: ✅ Compatible (basic server URL)
- **Note**: The current implementation encodes only the server URL. For advanced device configuration (tracking intervals, accuracy, etc.), additional parameters can be added to the QR code format.

## How to Use

### For End Users

1. **Open Traccar Manager app** on your mobile device
2. **Navigate to server settings** or open the app if not configured
3. **Tap the QR code scan button** in the app
4. **On the Motus login page**, click the QR code icon in the top-right corner
5. **Scan the displayed QR code** with the Traccar Manager app
6. **Server URL is automatically configured** in the app
7. **Log in** with your credentials

### For Administrators

The QR code is automatically generated based on your Motus server URL. No configuration is required.

## Technical Details

### QR Code Format

**Basic Format (Current Implementation)**:
```
https://your-motus-server.com
```

The QR code encodes the server URL in plain text. This is compatible with Traccar Manager's QR code scanning feature introduced in Traccar 6.8.

### Frontend Implementation

- **Location**: Login page (`web/src/routes/login/+page.svelte`)
- **Component**: QR Code Dialog (`web/src/lib/components/QrCodeDialog.svelte`)
- **Library**: Uses `qrcode` npm package for client-side QR code generation
- **No Backend Changes**: This is a pure frontend feature requiring no API modifications

### QR Code Properties

- **Size**: 256x256 pixels
- **Margin**: 2 modules
- **Color**: Black on white (standard)
- **Format**: PNG rendered on HTML canvas
- **Encoding**: UTF-8 plain text (server URL)

## Security Considerations

### What is NOT included in the QR code:
- ❌ User credentials (username/password)
- ❌ Authentication tokens
- ❌ API keys
- ❌ Session cookies

### Authentication Flow:
1. User scans QR code → Server URL configured
2. User must still log in with username/password or API token
3. This prevents unauthorized access via QR code alone

### Best Practices:
- Display the QR code only on trusted devices
- Don't share QR code screenshots publicly (they contain your server URL)
- Use HTTPS for production deployments
- Rotate API tokens regularly for programmatic access

## Testing

### Unit Tests
```bash
cd web
npm run test:unit -- QrCodeDialog.test.ts
```

### E2E Tests
```bash
cd web
npm run test -- qr-code-login.spec.ts
```

### Manual Testing Checklist
- [ ] QR code button visible on login page
- [ ] Clicking button opens dialog
- [ ] QR code is displayed correctly
- [ ] Server URL is shown below QR code
- [ ] Clicking server URL selects text for copying
- [ ] Dialog closes when clicking X button
- [ ] Dialog closes when clicking Close button
- [ ] Dialog closes when clicking backdrop
- [ ] QR code is scannable with Traccar Manager app
- [ ] After scanning, server URL is configured in app
- [ ] Login still works normally after scanning

## Future Enhancements

### Advanced Configuration QR Codes

For Traccar Client (device tracking app), QR codes can include additional parameters:

```
traccar://client?url=https://your-server.com&id=device123&accuracy=highest&interval=300
```

**Possible Parameters**:
- `id`: Device identifier
- `accuracy`: GPS accuracy level (highest, high, medium, low)
- `distance`: Distance filter (meters)
- `interval`: Location update interval (seconds)
- `angle`: Angle filter (degrees)
- `heartbeat`: Heartbeat interval (milliseconds)
- `buffer`: Enable location buffering (true/false)
- `wakelock`: Keep device awake (true/false)
- `stop_detection`: Enable stop detection (true/false)

### Pre-authenticated QR Codes

Generate QR codes with temporary authentication tokens:

```
https://your-server.com?token=temp_xxxxxxxxxxxx
```

This would require:
1. Backend endpoint to generate short-lived tokens
2. Token validation in authentication middleware
3. Automatic token cleanup after expiration

## Troubleshooting

### QR Code Not Displaying
- Check browser console for JavaScript errors
- Ensure `qrcode` npm package is installed
- Verify canvas element is present in DOM

### QR Code Not Scannable
- Ensure adequate screen brightness
- Try increasing QR code size (modify `width` in component)
- Verify QR code contains valid URL
- Check for special characters in server URL

### Server URL Incorrect
- Verify `window.location.origin` returns correct URL
- Check proxy/reverse proxy configuration
- Ensure HTTPS is properly configured for production

## References

- [Traccar 6.8 Release Notes](https://www.traccar.org/blog/traccar-6-8/)
- [QR Code Configuration Forum Thread](https://www.traccar.org/forums/topic/qr-code-configuration/)
- [Traccar Manager GitHub](https://github.com/traccar/traccar-manager)
- [QR Code npm Package](https://www.npmjs.com/package/qrcode)

## Compatibility Matrix

| Client | Minimum Version | Status | Notes |
|--------|----------------|--------|-------|
| Traccar Manager iOS | 6.8+ | ✅ Supported | WebView-based, displays web interface |
| Traccar Manager Android | 6.8+ | ✅ Supported | WebView-based, displays web interface |
| Traccar Client iOS | Any | ✅ Compatible | Basic server URL only |
| Traccar Client Android | Any | ✅ Compatible | Basic server URL only |
| Traccar Web | Any | ✅ Supported | Native feature |
| Home Assistant Integration | Any | N/A | Uses API tokens, not QR codes |

## Changelog

### 2026-02-17 - Initial Release
- Added QR code button to login page
- Implemented QR code dialog component
- Server URL encoding in QR code
- Full Traccar Manager compatibility
- Unit and E2E tests
- Documentation
