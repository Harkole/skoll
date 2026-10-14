package api

import (
	"html/template"
)

var (
	linkSuccessTemplate = template.Must(template.New("link_success").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sköll - Account Linked</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;600;700&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0d1117;
            --surface-color: #161b22;
            --border-color: #30363d;
            --text-primary: #f0f6fc;
            --text-secondary: #8b949e;
            --accent-glow: rgba(56, 189, 248, 0.15);
            --accent-cyan: #38bdf8;
            --accent-green: #3fb950;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'Inter', sans-serif;
            background-color: var(--bg-color);
            color: var(--text-primary);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 24px;
        }
        .card {
            background: var(--surface-color);
            border: 1px solid var(--border-color);
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5), 0 0 40px var(--accent-glow);
            border-radius: 16px;
            max-width: 520px;
            width: 100%;
            padding: 40px;
            text-align: center;
            backdrop-filter: blur(8px);
        }
        .icon {
            width: 64px;
            height: 64px;
            border-radius: 50%;
            background: rgba(63, 185, 80, 0.15);
            color: var(--accent-green);
            display: inline-flex;
            align-items: center;
            justify-content: center;
            margin-bottom: 24px;
            font-size: 32px;
            box-shadow: 0 0 20px rgba(63, 185, 80, 0.3);
        }
        h1 {
            font-size: 26px;
            font-weight: 700;
            margin-bottom: 12px;
            letter-spacing: -0.5px;
        }
        p {
            color: var(--text-secondary);
            font-size: 15px;
            line-height: 1.6;
            margin-bottom: 24px;
        }
        .info-pill {
            display: inline-block;
            background: #21262d;
            border: 1px solid #30363d;
            border-radius: 8px;
            padding: 10px 18px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 14px;
            color: var(--accent-cyan);
            margin-bottom: 24px;
        }
        .role-badge {
            background: rgba(56, 189, 248, 0.15);
            color: #38bdf8;
            padding: 4px 10px;
            border-radius: 12px;
            font-weight: 600;
            font-size: 13px;
        }
        .btn {
            display: inline-block;
            background: linear-gradient(135deg, #2563eb, #1d4ed8);
            color: white;
            padding: 12px 28px;
            border-radius: 8px;
            text-decoration: none;
            font-weight: 600;
            font-size: 14px;
            transition: all 0.2s ease;
        }
        .btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 16px rgba(37, 99, 235, 0.4);
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">✓</div>
        <h1>Identity Verified</h1>
        <p>Your Discord account has been securely linked to the Fenrir Daemon ecosystem.</p>
        <div class="info-pill">
            <span>{{.Username}}</span> &bull; Role: <span class="role-badge">{{.Role}}</span>
        </div>
        <p>You can now close this tab and execute authorized commands directly within Discord!</p>
    </div>
</body>
</html>`))

	errorTemplate = template.Must(template.New("error").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sköll - Authentication Error</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" rel="stylesheet">
    <style>
        body {
            font-family: 'Inter', sans-serif;
            background-color: #0d1117;
            color: #f0f6fc;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .card {
            background: #161b22;
            border: 1px solid #f85149;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.6);
            border-radius: 16px;
            max-width: 480px;
            width: 100%;
            padding: 36px;
            text-align: center;
        }
        .icon {
            font-size: 36px;
            color: #f85149;
            margin-bottom: 16px;
        }
        h1 { font-size: 22px; margin-bottom: 12px; }
        p { color: #8b949e; line-height: 1.5; font-size: 14px; margin-bottom: 24px; }
        .details {
            background: #21262d;
            border-radius: 6px;
            padding: 12px;
            font-family: monospace;
            font-size: 13px;
            color: #ff7b72;
            word-break: break-all;
            margin-bottom: 20px;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">⚠</div>
        <h1>Authentication Failed</h1>
        <p>An error occurred while linking your Discord account with Sköll.</p>
        <div class="details">{{.ErrorMessage}}</div>
        <p>Please return to Discord and run <code>/link</code> again to receive a fresh token.</p>
    </div>
</body>
</html>`))

	adminTemplate = template.Must(template.New("admin").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sköll - Admin Control Panel</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;600&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0b0f17;
            --surface-color: #121824;
            --surface-hover: #1b2333;
            --border-color: #232d3f;
            --border-light: #2e3a52;
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent-blue: #38bdf8;
            --accent-purple: #a855f7;
            --accent-green: #22c55e;
            --accent-amber: #f59e0b;
            --accent-red: #ef4444;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: 'Inter', sans-serif;
            background-color: var(--bg-color);
            color: var(--text-primary);
            min-height: 100vh;
            padding: 32px 24px;
        }
        .container {
            max-width: 1280px;
            margin: 0 auto;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 32px;
            padding-bottom: 20px;
            border-bottom: 1px solid var(--border-color);
        }
        .logo {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        .logo-symbol {
            width: 36px;
            height: 36px;
            background: linear-gradient(135deg, #0284c7, #6366f1);
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            font-size: 18px;
            color: white;
            box-shadow: 0 0 15px rgba(2, 132, 199, 0.4);
        }
        .logo h1 {
            font-size: 22px;
            font-weight: 700;
            letter-spacing: -0.5px;
        }
        .badge-system {
            font-size: 12px;
            background: #1e293b;
            color: var(--accent-blue);
            padding: 4px 10px;
            border-radius: 6px;
            border: 1px solid #334155;
            font-family: 'JetBrains Mono', monospace;
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
            gap: 16px;
            margin-bottom: 32px;
        }
        .stat-card {
            background: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 20px;
            position: relative;
            overflow: hidden;
        }
        .stat-title {
            color: var(--text-secondary);
            font-size: 13px;
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 8px;
        }
        .stat-value {
            font-size: 28px;
            font-weight: 700;
            color: var(--text-primary);
        }
        .section-title {
            font-size: 18px;
            font-weight: 600;
            margin-bottom: 16px;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .card {
            background: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            overflow: hidden;
            margin-bottom: 36px;
            box-shadow: 0 4px 20px rgba(0, 0, 0, 0.2);
        }
        table {
            width: 100%;
            border-collapse: collapse;
            text-align: left;
        }
        th {
            background: #0f1522;
            padding: 14px 18px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            color: var(--text-secondary);
            border-bottom: 1px solid var(--border-color);
        }
        td {
            padding: 14px 18px;
            font-size: 14px;
            border-bottom: 1px solid var(--border-color);
            vertical-align: middle;
        }
        tr:last-child td {
            border-bottom: none;
        }
        tr:hover td {
            background-color: var(--surface-hover);
        }
        .role-pill {
            display: inline-block;
            padding: 3px 10px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .role-Admin { background: rgba(168, 85, 247, 0.15); color: #c084fc; border: 1px solid rgba(168, 85, 247, 0.3); }
        .role-Operator { background: rgba(245, 158, 11, 0.15); color: #fbbf24; border: 1px solid rgba(245, 158, 11, 0.3); }
        .role-Viewer { background: rgba(34, 197, 94, 0.15); color: #4ade80; border: 1px solid rgba(34, 197, 94, 0.3); }
        .role-Unlinked { background: rgba(148, 163, 184, 0.15); color: #94a3b8; border: 1px solid rgba(148, 163, 184, 0.3); }

        .status-pill {
            display: inline-block;
            padding: 2px 8px;
            border-radius: 4px;
            font-size: 11px;
            font-weight: 600;
            font-family: 'JetBrains Mono', monospace;
        }
        .status-SUCCESS { background: rgba(34, 197, 94, 0.2); color: #4ade80; }
        .status-FAILED { background: rgba(239, 68, 68, 0.2); color: #f87171; }
        .status-UNAUTHORIZED { background: rgba(245, 158, 11, 0.2); color: #fbbf24; }

        .mono {
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
        }
        select {
            background: #0f1522;
            color: var(--text-primary);
            border: 1px solid var(--border-light);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 13px;
            cursor: pointer;
            outline: none;
        }
        select:focus {
            border-color: var(--accent-blue);
        }
        .btn-update {
            background: #2563eb;
            color: white;
            border: none;
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            transition: background 0.15s ease;
            margin-left: 6px;
        }
        .btn-update:hover {
            background: #1d4ed8;
        }
        .alert-msg {
            padding: 12px 18px;
            border-radius: 8px;
            margin-bottom: 24px;
            font-size: 14px;
            background: rgba(34, 197, 94, 0.15);
            color: #4ade80;
            border: 1px solid rgba(34, 197, 94, 0.3);
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">
                <div class="logo-symbol">S</div>
                <div>
                    <h1>Sköll Companion</h1>
                    <p style="font-size: 13px; color: var(--text-secondary);">Fenrir Daemon Access Control & Bot Gateway</p>
                </div>
            </div>
            <div style="display: flex; gap: 12px; align-items: center;">
                <span class="badge-system">Fenrir: {{.FenrirStatus}}</span>
            </div>
        </header>

        {{if .Notification}}
        <div class="alert-msg">
            {{.Notification}}
        </div>
        {{end}}

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-title">Linked Users</div>
                <div class="stat-value">{{.TotalUsers}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-title">Administrators</div>
                <div class="stat-value">{{.AdminCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-title">Operators</div>
                <div class="stat-value">{{.OperatorCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-title">Audited Actions</div>
                <div class="stat-value">{{.AuditCount}}</div>
            </div>
        </div>

        <div class="section-title">
            <span>Linked Users & Role Management</span>
        </div>
        <div class="card">
            <table>
                <thead>
                    <tr>
                        <th>User ID</th>
                        <th>Discord ID</th>
                        <th>Username</th>
                        <th>Current Role</th>
                        <th>Linked At</th>
                        <th>Update Role</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Users}}
                    <tr>
                        <td class="mono">#{{.ID}}</td>
                        <td class="mono">{{.DiscordID}}</td>
                        <td><strong>{{.Username}}</strong></td>
                        <td><span class="role-pill role-{{.Role}}">{{.Role}}</span></td>
                        <td class="mono" style="color: var(--text-secondary); font-size: 12px;">{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
                        <td>
                            <form method="POST" action="/admin/role" style="display: inline-flex; align-items: center;">
                                <input type="hidden" name="user_id" value="{{.ID}}">
                                <input type="hidden" name="key" value="{{$.AdminKey}}">
                                <select name="role">
                                    <option value="Admin" {{if eq .Role "Admin"}}selected{{end}}>Admin</option>
                                    <option value="Operator" {{if eq .Role "Operator"}}selected{{end}}>Operator</option>
                                    <option value="Viewer" {{if eq .Role "Viewer"}}selected{{end}}>Viewer</option>
                                    <option value="Unlinked" {{if eq .Role "Unlinked"}}selected{{end}}>Unlinked</option>
                                </select>
                                <button type="submit" class="btn-update">Save</button>
                            </form>
                        </td>
                    </tr>
                    {{else}}
                    <tr>
                        <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 32px;">
                            No users have linked their Discord account yet. Run <code>/link</code> in your Discord server.
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>

        <div class="section-title">
            <span>Recent Activity Audit Feed</span>
        </div>
        <div class="card">
            <table>
                <thead>
                    <tr>
                        <th>Timestamp (UTC)</th>
                        <th>User</th>
                        <th>Discord ID</th>
                        <th>Action</th>
                        <th>Target Stack</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .AuditLogs}}
                    <tr>
                        <td class="mono" style="color: var(--text-secondary); font-size: 12px;">{{.Timestamp.Format "2006-01-02 15:04:05"}}</td>
                        <td><strong>{{.Username}}</strong></td>
                        <td class="mono" style="color: var(--text-secondary);">{{.DiscordUserID}}</td>
                        <td><span class="mono" style="color: var(--accent-blue);">{{.Action}}</span></td>
                        <td class="mono">{{.TargetProject}}</td>
                        <td><span class="status-pill status-{{.Status}}">{{.Status}}</span></td>
                    </tr>
                    {{else}}
                    <tr>
                        <td colspan="6" style="text-align: center; color: var(--text-secondary); padding: 32px;">
                            No audit activity recorded yet.
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>
</body>
</html>`))

	adminLoginTemplate = template.Must(template.New("admin_login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sköll Admin - Authorization Required</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        body {
            font-family: 'Inter', sans-serif;
            background-color: #0b0f17;
            color: #f8fafc;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 24px;
        }
        .login-card {
            background: #121824;
            border: 1px solid #232d3f;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
            border-radius: 14px;
            max-width: 420px;
            width: 100%;
            padding: 36px;
            text-align: center;
        }
        h2 { font-size: 22px; margin-bottom: 8px; }
        p { color: #94a3b8; font-size: 14px; margin-bottom: 24px; }
        input[type="password"] {
            width: 100%;
            background: #0f1522;
            border: 1px solid #334155;
            color: white;
            padding: 12px 14px;
            border-radius: 8px;
            font-size: 14px;
            margin-bottom: 16px;
            outline: none;
        }
        input[type="password"]:focus {
            border-color: #38bdf8;
        }
        button {
            width: 100%;
            background: #2563eb;
            color: white;
            border: none;
            padding: 12px;
            border-radius: 8px;
            font-weight: 600;
            cursor: pointer;
        }
        button:hover { background: #1d4ed8; }
    </style>
</head>
<body>
    <div class="login-card">
        <h2>Sköll Admin Panel</h2>
        <p>Please enter your <code>SKOLL_ADMIN_KEY</code> to access user role management.</p>
        <form method="GET" action="/admin">
            <input type="password" name="key" placeholder="Enter Admin Secret Key" required autofocus>
            <button type="submit">Access Dashboard</button>
        </form>
    </div>
</body>
</html>`))
)
