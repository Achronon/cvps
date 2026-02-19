# Authentication

The CVPS CLI supports two authentication methods: OAuth (browser-based) and API keys.

## Login

### OAuth Authentication (Recommended)

The default authentication method opens your browser for a secure OAuth flow:

```bash
cvps login
```

When prompted, choose option `1` (or press Enter for default). The CLI will:
1. Open your browser to the ClaudeVPS authentication page
2. Display a verification code
3. Wait for you to complete authentication in the browser
4. Save your access token securely

### API Key Authentication

If you prefer to use an API key, you have two options:

**Option 1: Command-line flag**
```bash
cvps login --api-key YOUR_API_KEY
```

**Option 2: Interactive prompt**
```bash
cvps login
# Choose option 2 when prompted
# Enter your API key when requested
```

## Check Current User

To verify your authentication and see your current user:

```bash
cvps whoami
```

Output:
```
Logged in as: John Doe (john@example.com)
User ID: user-abc123
```

## Logout

To clear your stored credentials:

```bash
cvps logout
```

## Authentication Details

### Storage
Credentials are stored in `~/.cvps/config.yaml` with file permissions set to `0600` (read/write for owner only) for security.

### Token Types
- **OAuth tokens**: Access tokens with optional refresh tokens
- **API keys**: Long-lived keys for automation and CI/CD

### Preference Order
If both an OAuth token and API key are present, the OAuth token takes precedence.

### Environment Variables

You can override authentication via environment variables:

- `CVPS_API_KEY`: Set your API key
- `CVPS_API_URL`: Override the API base URL (default: https://api.claudevps.com)

Example:
```bash
export CVPS_API_KEY=your-api-key
cvps whoami  # Uses the environment variable
```

## Security Best Practices

1. **Never share your API key or tokens**
2. **Use OAuth for interactive sessions** - More secure with automatic expiration
3. **Use API keys for automation** - Required for CI/CD and scripts
4. **Rotate API keys regularly** - Generate new keys periodically
5. **Use environment variables in CI/CD** - Don't commit credentials to version control

## Troubleshooting

### "not logged in" error
Run `cvps login` to authenticate.

### "invalid API key" error
- Check that your API key is correct
- Generate a new API key from the ClaudeVPS dashboard
- Ensure the API key hasn't been revoked

### OAuth timeout
The device authorization flow expires after 5 minutes. If you see a timeout error:
1. Run `cvps login` again
2. Complete the browser authentication more quickly
3. Check your internet connection

### Browser doesn't open automatically
If the browser doesn't open:
1. The CLI will display a URL and verification code
2. Manually open the URL in your browser
3. Enter the verification code when prompted
