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
            margin-bottom: 24px;
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
            margin-bottom: 28px;
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
            font-size: 26px;
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
            padding: 3px 10px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            font-family: 'JetBrains Mono', monospace;
        }
        .status-running, .status-SUCCESS { background: rgba(34, 197, 94, 0.15); color: #4ade80; border: 1px solid rgba(34, 197, 94, 0.3); }
        .status-stopped, .status-FAILED { background: rgba(239, 68, 68, 0.15); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.3); }
        .status-degraded, .status-UNAUTHORIZED { background: rgba(245, 158, 11, 0.15); color: #fbbf24; border: 1px solid rgba(245, 158, 11, 0.3); }
        .status-unknown { background: rgba(148, 163, 184, 0.15); color: #94a3b8; border: 1px solid rgba(148, 163, 184, 0.3); }

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

        /* Navigation Tabs */
        .nav-tabs {
            display: flex;
            gap: 10px;
            margin-bottom: 24px;
            border-bottom: 1px solid var(--border-color);
            padding-bottom: 12px;
        }
        .nav-tab {
            background: transparent;
            border: 1px solid transparent;
            color: var(--text-secondary);
            padding: 9px 18px;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
            display: inline-flex;
            align-items: center;
            gap: 8px;
        }
        .nav-tab:hover {
            color: var(--text-primary);
            background: var(--surface-hover);
        }
        .nav-tab.active {
            background: #1e293b;
            color: var(--accent-blue);
            border-color: var(--border-light);
            font-weight: 600;
            box-shadow: 0 0 15px rgba(56, 189, 248, 0.1);
        }
        .tab-panel {
            display: none;
        }
        .tab-panel.active {
            display: block;
            animation: tabFadeIn 0.2s ease;
        }
        @keyframes tabFadeIn {
            from { opacity: 0; transform: translateY(4px); }
            to { opacity: 1; transform: translateY(0); }
        }

        /* Services display inside stack table */
        .service-list {
            display: flex;
            flex-direction: column;
            gap: 6px;
            max-width: 480px;
        }
        .service-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 13px;
            background: rgba(15, 21, 34, 0.6);
            padding: 6px 10px;
            border-radius: 6px;
            border: 1px solid rgba(46, 58, 82, 0.5);
            flex-wrap: wrap;
        }
        .service-state-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            display: inline-block;
            flex-shrink: 0;
        }
        .dot-running { background-color: var(--accent-green); box-shadow: 0 0 6px rgba(34, 197, 94, 0.6); }
        .dot-stopped, .dot-exited { background-color: var(--accent-red); box-shadow: 0 0 6px rgba(239, 68, 68, 0.6); }
        .dot-other { background-color: var(--accent-amber); box-shadow: 0 0 6px rgba(245, 158, 11, 0.6); }
        .service-image {
            color: var(--text-secondary);
            font-family: 'JetBrains Mono', monospace;
            font-size: 12px;
        }
        .service-port {
            background: #1e293b;
            color: var(--accent-blue);
            padding: 2px 6px;
            border-radius: 4px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 11px;
        }

        .btn-view-compose {
            background: #1e293b;
            color: var(--accent-blue);
            border: 1px solid #334155;
            padding: 7px 14px;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            transition: all 0.15s ease;
        }
        .btn-view-compose:hover {
            background: #2e3a52;
            border-color: var(--accent-blue);
            color: white;
            box-shadow: 0 0 12px rgba(56, 189, 248, 0.2);
        }

        /* Modal Overlay & Card */
        .modal-overlay {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(5, 8, 14, 0.85);
            backdrop-filter: blur(5px);
            z-index: 1000;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .modal-overlay.open {
            display: flex;
        }
        .modal-card {
            background: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 14px;
            width: 100%;
            max-width: 860px;
            max-height: 85vh;
            display: flex;
            flex-direction: column;
            box-shadow: 0 20px 50px rgba(0, 0, 0, 0.7), 0 0 40px rgba(56, 189, 248, 0.1);
            overflow: hidden;
            animation: modalFadeIn 0.2s ease-out;
        }
        @keyframes modalFadeIn {
            from { opacity: 0; transform: scale(0.97); }
            to { opacity: 1; transform: scale(1); }
        }
        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 16px 22px;
            background: #0f1522;
            border-bottom: 1px solid var(--border-color);
        }
        .modal-header h3 {
            font-size: 16px;
            font-weight: 600;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .modal-body {
            padding: 20px;
            overflow-y: auto;
            flex: 1;
        }
        .modal-footer {
            display: flex;
            justify-content: flex-end;
            gap: 10px;
            padding: 14px 22px;
            background: #0f1522;
            border-top: 1px solid var(--border-color);
        }
        .code-preview {
            background: #090d14;
            border: 1px solid #1e293b;
            border-radius: 8px;
            padding: 18px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
            line-height: 1.6;
            color: #e2e8f0;
            white-space: pre-wrap;
            word-break: break-all;
            user-select: text;
        }
        .btn-copy {
            background: #1e293b;
            color: var(--text-primary);
            border: 1px solid var(--border-light);
            padding: 7px 14px;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            transition: all 0.15s ease;
        }
        .btn-copy:hover {
            background: #2e3a52;
            color: var(--accent-blue);
        }
        .btn-close {
            background: transparent;
            color: var(--text-secondary);
            border: none;
            font-size: 22px;
            cursor: pointer;
            line-height: 1;
            padding: 4px;
        }
        .btn-close:hover {
            color: var(--text-primary);
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
                <div class="stat-title">Docker Stacks</div>
                <div class="stat-value">{{.RunningStacksCount}} / {{.TotalStacks}} <span style="font-size: 14px; font-weight: normal; color: var(--accent-green);">Active</span></div>
            </div>
            <div class="stat-card">
                <div class="stat-title">Linked Users</div>
                <div class="stat-value">{{.TotalUsers}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-title">Administrators</div>
                <div class="stat-value">{{.AdminCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-title">Audited Actions</div>
                <div class="stat-value">{{.AuditCount}}</div>
            </div>
        </div>

        <!-- Navigation Tabs -->
        <div class="nav-tabs">
            <button type="button" id="tab-btn-stacks" class="nav-tab active" onclick="switchTab('stacks')">
                <span>🐳</span> Docker Compose Stacks ({{len .Projects}})
            </button>
            <button type="button" id="tab-btn-users" class="nav-tab" onclick="switchTab('users')">
                <span>👥</span> Users & Role RBAC ({{.TotalUsers}})
            </button>
            <button type="button" id="tab-btn-audit" class="nav-tab" onclick="switchTab('audit')">
                <span>📜</span> Audit Activity ({{.AuditCount}})
            </button>
        </div>

        <!-- Stacks Tab Panel -->
        <div id="tab-panel-stacks" class="tab-panel active">
            <div class="section-title">
                <span>Docker Compose Stacks & Compose Configurations</span>
            </div>
            <div class="card">
                <table>
                    <thead>
                        <tr>
                            <th>Stack Name</th>
                            <th>Status</th>
                            <th>Services & Containers</th>
                            <th>Created (UTC)</th>
                            <th>Compose Definition</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .Projects}}
                        <tr>
                            <td>
                                <strong style="font-size: 15px; color: var(--accent-blue);">{{.Name}}</strong>
                            </td>
                            <td>
                                <span class="status-pill status-{{.Status}}">{{.Status}}</span>
                            </td>
                            <td>
                                <div class="service-list">
                                    {{range .Services}}
                                    <div class="service-item">
                                        <span class="service-state-dot {{if eq .State "running"}}dot-running{{else if eq .State "exited"}}dot-stopped{{else}}dot-other{{end}}"></span>
                                        <strong>{{.Name}}</strong>
                                        {{if .Image}}<span class="service-image">{{.Image}}</span>{{end}}
                                        {{range .Ports}}<span class="service-port">{{.}}</span>{{end}}
                                        {{if .Status}}<span style="color: var(--text-secondary); font-size: 11px;">({{.Status}})</span>{{end}}
                                    </div>
                                    {{else}}
                                    <span style="color: var(--text-secondary); font-size: 13px;"><em>No active services</em></span>
                                    {{end}}
                                </div>
                            </td>
                            <td class="mono" style="color: var(--text-secondary); font-size: 12px;">
                                {{if not .CreatedAt.IsZero}}{{.CreatedAt.Format "2006-01-02 15:04:05"}}{{else}}-{{end}}
                            </td>
                            <td>
                                <textarea id="compose-data-{{.Name}}" style="display:none;">{{.Compose}}</textarea>
                                <button type="button" class="btn-view-compose" onclick="showComposeModal('{{.Name}}')">
                                    <span>📄</span> View compose.yml
                                </button>
                            </td>
                        </tr>
                        {{else}}
                        <tr>
                            <td colspan="5" style="text-align: center; color: var(--text-secondary); padding: 40px;">
                                <div style="font-size: 24px; margin-bottom: 8px;">🐳</div>
                                <div style="font-size: 15px; margin-bottom: 4px;">No Docker Compose stacks found in Fenrir Daemon.</div>
                                <div style="font-size: 13px;">Deploy a new stack from Discord using <code>/stack deploy &lt;name&gt; &lt;compose&gt;</code>.</div>
                            </td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
        </div>

        <!-- Users Tab Panel -->
        <div id="tab-panel-users" class="tab-panel">
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
        </div>

        <!-- Audit Tab Panel -->
        <div id="tab-panel-audit" class="tab-panel">
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
    </div>

    <!-- Compose YAML Viewer Modal -->
    <div id="composeModal" class="modal-overlay" onclick="onModalOverlayClick(event)">
        <div class="modal-card">
            <div class="modal-header">
                <h3>
                    <span>📄</span>
                    <span>compose.yml</span>
                    <span id="modalStackName" class="badge-system"></span>
                </h3>
                <div style="display: flex; gap: 8px; align-items: center;">
                    <button type="button" id="btnCopy" class="btn-copy" onclick="copyComposeYAML()">📋 Copy YAML</button>
                    <button type="button" class="btn-close" onclick="closeComposeModal()">&times;</button>
                </div>
            </div>
            <div class="modal-body">
                <pre class="code-preview"><code id="modalComposeContent">Loading compose specification...</code></pre>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn-copy" onclick="closeComposeModal()">Close</button>
            </div>
        </div>
    </div>

    <script>
        function switchTab(tabName) {
            // Update tab button classes
            const tabs = ['stacks', 'users', 'audit'];
            tabs.forEach(t => {
                const btn = document.getElementById('tab-btn-' + t);
                const panel = document.getElementById('tab-panel-' + t);
                if (btn && panel) {
                    if (t === tabName) {
                        btn.classList.add('active');
                        panel.classList.add('active');
                    } else {
                        btn.classList.remove('active');
                        panel.classList.remove('active');
                    }
                }
            });
            try {
                if (history.replaceState) {
                    history.replaceState(null, '', '#' + tabName);
                }
            } catch (e) {}
        }

        function showComposeModal(stackName) {
            const modal = document.getElementById('composeModal');
            const nameEl = document.getElementById('modalStackName');
            const codeEl = document.getElementById('modalComposeContent');
            const copyBtn = document.getElementById('btnCopy');

            nameEl.textContent = stackName;
            copyBtn.textContent = '📋 Copy YAML';

            const textarea = document.getElementById('compose-data-' + stackName);
            if (textarea && textarea.value && textarea.value.trim() !== '') {
                codeEl.textContent = textarea.value;
                modal.classList.add('open');
            } else {
                codeEl.textContent = 'Fetching compose definition from Fenrir Daemon...';
                modal.classList.add('open');
                
                // Fetch dynamically via REST fallback
                fetch('/admin/compose?name=' + encodeURIComponent(stackName) + '&key=' + encodeURIComponent('{{.AdminKey}}'))
                    .then(r => {
                        if (!r.ok) throw new Error('HTTP ' + r.status);
                        return r.json();
                    })
                    .then(data => {
                        codeEl.textContent = data.compose || '# No compose definition found.';
                    })
                    .catch(err => {
                        codeEl.textContent = '# Failed to fetch compose definition: ' + err.message;
                    });
            }
        }

        function closeComposeModal() {
            const modal = document.getElementById('composeModal');
            modal.classList.remove('open');
        }

        function onModalOverlayClick(e) {
            if (e.target && e.target.id === 'composeModal') {
                closeComposeModal();
            }
        }

        function copyComposeYAML() {
            const codeEl = document.getElementById('modalComposeContent');
            const copyBtn = document.getElementById('btnCopy');
            if (codeEl) {
                navigator.clipboard.writeText(codeEl.textContent).then(() => {
                    copyBtn.textContent = '✓ Copied!';
                    setTimeout(() => {
                        copyBtn.textContent = '📋 Copy YAML';
                    }, 2000);
                }).catch(() => {
                    copyBtn.textContent = 'Error copying';
                });
            }
        }

        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                closeComposeModal();
            }
        });

        // Initialize active tab from URL hash or server preference
        window.addEventListener('DOMContentLoaded', () => {
            const hash = (window.location.hash || '').replace('#', '');
            if (hash === 'users' || hash === 'audit' || hash === 'stacks') {
                switchTab(hash);
            } else {
                switchTab('{{.ActiveTab}}' || 'stacks');
            }
        });
    </script>
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
