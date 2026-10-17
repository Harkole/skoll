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

        /* Editor Modal Enhancements */
        .modal-subnav {
            background: #0f1522;
            padding: 12px 22px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            flex-direction: column;
            gap: 10px;
        }
        .editor-tabs {
            display: flex;
            gap: 8px;
        }
        .editor-tab {
            background: transparent;
            border: 1px solid transparent;
            color: var(--text-secondary);
            padding: 6px 14px;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.15s ease;
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        .editor-tab:hover {
            color: var(--text-primary);
            background: var(--surface-hover);
        }
        .editor-tab.active {
            background: #1e293b;
            color: var(--accent-blue);
            border-color: var(--border-light);
            font-weight: 600;
        }
        .mode-controls {
            display: flex;
            align-items: center;
            justify-content: space-between;
            font-size: 13px;
            color: var(--text-secondary);
        }
        .control-label {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            cursor: pointer;
            font-size: 12px;
            color: var(--text-secondary);
        }
        .control-label input[type="checkbox"] {
            cursor: pointer;
        }
        .input-filename {
            background: #090d14;
            border: 1px solid var(--border-light);
            color: var(--text-primary);
            padding: 6px 12px;
            border-radius: 6px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 12px;
            width: 240px;
            outline: none;
        }
        .input-filename:focus {
            border-color: var(--accent-blue);
        }
        .btn-load-aux {
            background: #1e293b;
            color: var(--accent-blue);
            border: 1px solid var(--border-light);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 12px;
            cursor: pointer;
            font-weight: 500;
            transition: all 0.15s ease;
        }
        .btn-load-aux:hover {
            background: #2e3a52;
            color: white;
        }
        .editor-wrapper {
            display: flex;
            flex-direction: column;
            width: 100%;
        }
        .code-editor {
            width: 100%;
            min-height: 400px;
            height: 52vh;
            background: #090d14;
            color: #e2e8f0;
            border: 1px solid #1e293b;
            border-radius: 8px;
            padding: 16px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
            line-height: 1.6;
            resize: vertical;
            outline: none;
            transition: border-color 0.2s ease, box-shadow 0.2s ease;
            tab-size: 2;
        }
        .code-editor:focus {
            border-color: var(--accent-blue);
            box-shadow: 0 0 15px rgba(56, 189, 248, 0.15);
        }
        .editor-alert {
            padding: 10px 16px;
            border-radius: 8px;
            font-size: 13px;
            margin-bottom: 12px;
            line-height: 1.5;
            animation: tabFadeIn 0.2s ease;
        }
        .editor-alert.success {
            background: rgba(34, 197, 94, 0.15);
            color: #4ade80;
            border: 1px solid rgba(34, 197, 94, 0.3);
        }
        .editor-alert.error {
            background: rgba(239, 68, 68, 0.15);
            color: #f87171;
            border: 1px solid rgba(239, 68, 68, 0.3);
        }
        .editor-alert.info {
            background: rgba(56, 189, 248, 0.15);
            color: #38bdf8;
            border: 1px solid rgba(56, 189, 248, 0.3);
        }
        .btn-save {
            background: linear-gradient(135deg, #0284c7, #2563eb);
            color: white;
            border: none;
            padding: 8px 18px;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s ease;
            box-shadow: 0 0 15px rgba(2, 132, 199, 0.3);
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        .btn-save:hover {
            transform: translateY(-1px);
            box-shadow: 0 4px 18px rgba(2, 132, 199, 0.5);
        }
        .btn-dryrun {
            background: rgba(245, 158, 11, 0.15);
            color: #fbbf24;
            border: 1px solid rgba(245, 158, 11, 0.4);
            padding: 8px 16px;
            border-radius: 6px;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            display: inline-flex;
            align-items: center;
            gap: 6px;
            transition: all 0.2s ease;
        }
        .btn-dryrun:hover {
            background: rgba(245, 158, 11, 0.25);
            color: #fcd34d;
        }
        .editor-status-pill {
            font-size: 11px;
            font-family: 'JetBrains Mono', monospace;
            padding: 3px 8px;
            border-radius: 4px;
            background: #1e293b;
            color: var(--text-secondary);
        }
        .editor-meta {
            font-size: 12px;
            color: var(--text-secondary);
            font-family: 'JetBrains Mono', monospace;
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
                {{if .CurrentUser}}
                <div style="display: flex; align-items: center; gap: 8px; background: #161b22; padding: 6px 12px; border-radius: 8px; border: 1px solid #30363d;">
                    <span style="color: var(--accent-blue); font-weight: 600; font-size: 13px;">{{.CurrentUser.Username}}</span>
                    <span class="role-pill role-{{.CurrentUser.Role}}" style="font-size: 11px; padding: 2px 8px;">{{.CurrentUser.Role}}</span>
                    <a href="/auth/logout" style="color: var(--text-secondary); text-decoration: none; font-size: 12px; margin-left: 6px;" title="Logout">Log out</a>
                </div>
                {{end}}
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
                                <div style="display: flex; gap: 8px; align-items: center; flex-wrap: wrap;">
                                    <button type="button" class="btn-view-compose" onclick="showComposeModal('{{.Name}}', 'compose')">
                                        <span>📝</span> Edit Compose
                                    </button>
                                    <button type="button" class="btn-view-compose" style="color: #c084fc; border-color: rgba(168, 85, 247, 0.4);" onclick="showComposeModal('{{.Name}}', 'yaml')">
                                        <span>⚙️</span> Edit Config
                                    </button>
                                </div>
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

    <!-- Compose & Configuration YAML Editor Modal -->
    <div id="composeModal" class="modal-overlay" onclick="onModalOverlayClick(event)">
        <div class="modal-card">
            <div class="modal-header">
                <div style="display: flex; align-items: center; gap: 10px;">
                    <span style="font-size: 20px;">📝</span>
                    <div>
                        <h3 style="margin: 0; font-size: 16px;">Stack YAML Editor</h3>
                        <span id="modalStackName" class="badge-system" style="margin-top: 2px;"></span>
                    </div>
                </div>
                <div style="display: flex; gap: 8px; align-items: center;">
                    <button type="button" id="btnCopy" class="btn-copy" onclick="copyComposeYAML()">📋 Copy YAML</button>
                    <button type="button" class="btn-close" onclick="closeComposeModal()">&times;</button>
                </div>
            </div>

            <!-- Editor Mode Selector Tabs -->
            <div class="modal-subnav">
                <div class="editor-tabs">
                    <button type="button" id="mode-tab-compose" class="editor-tab active" onclick="switchEditorMode('compose')">
                        <span>🐳</span> docker-compose.yml
                    </button>
                    <button type="button" id="mode-tab-yaml" class="editor-tab" onclick="switchEditorMode('yaml')">
                        <span>⚙️</span> Auxiliary Config YAML
                    </button>
                </div>
                <!-- Controls for Compose mode -->
                <div id="composeControls" class="mode-controls">
                    <label class="control-label">
                        <input type="checkbox" id="composeRedeploy" checked>
                        <span>Auto-redeploy containers on save (<code>?redeploy=true</code>)</span>
                    </label>
                </div>
                <!-- Controls for Auxiliary YAML mode -->
                <div id="yamlControls" class="mode-controls" style="display: none;">
                    <div style="display: flex; gap: 8px; align-items: center; width: 100%; flex-wrap: wrap;">
                        <input type="text" id="auxFilename" class="input-filename" placeholder="e.g. config.yml, settings.yaml" value="config.yml">
                        <button type="button" class="btn-load-aux" onclick="loadAuxiliaryYAML()">📥 Load File</button>
                        <label class="control-label" style="margin-left: auto;">
                            <input type="checkbox" id="yamlRestart" checked>
                            <span>Restart stack on save (<code>?restart=true</code>)</span>
                        </label>
                    </div>
                </div>
            </div>

            <div class="modal-body">
                <!-- Status & Validation Alert Banner -->
                <div id="editorAlert" class="editor-alert" style="display: none;"></div>

                <div class="editor-wrapper">
                    <textarea id="modalComposeEditor" class="code-editor" spellcheck="false" placeholder="Loading YAML specification..."></textarea>
                    <!-- Fallback / Hidden mirror for backward compatibility -->
                    <pre class="code-preview" id="modalComposePreview" style="display: none;"><code id="modalComposeContent">Loading compose specification...</code></pre>
                </div>
            </div>

            <div class="modal-footer">
                <div style="display: flex; align-items: center; gap: 10px; margin-right: auto;">
                    <span id="editorStatusText" class="editor-status-pill">Ready</span>
                    <span id="editorMetaText" class="editor-meta">Lines: 1 | Chars: 0</span>
                </div>
                <button type="button" id="btnDryRun" class="btn-dryrun" onclick="validateComposeDryRun()">🧪 Validate (Dry Run)</button>
                <button type="button" id="btnSave" class="btn-save" onclick="saveYAMLContent()">💾 Save & Apply</button>
                <button type="button" class="btn-copy" onclick="closeComposeModal()">Close</button>
            </div>
        </div>
    </div>

    <script>
        let currentStackName = '';
        let currentEditorMode = 'compose'; // 'compose' or 'yaml'

        function switchTab(tabName) {
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

        function switchEditorMode(mode) {
            currentEditorMode = mode;
            const tabCompose = document.getElementById('mode-tab-compose');
            const tabYAML = document.getElementById('mode-tab-yaml');
            const composeControls = document.getElementById('composeControls');
            const yamlControls = document.getElementById('yamlControls');
            const btnDryRun = document.getElementById('btnDryRun');

            hideEditorAlert();

            if (mode === 'compose') {
                tabCompose.classList.add('active');
                tabYAML.classList.remove('active');
                composeControls.style.display = 'flex';
                yamlControls.style.display = 'none';
                btnDryRun.style.display = 'inline-flex';

                // Load compose content
                const textarea = document.getElementById('compose-data-' + currentStackName);
                if (textarea && textarea.value && textarea.value.trim() !== '') {
                    setEditorContent(textarea.value);
                } else {
                    fetchCompose(currentStackName);
                }
            } else {
                tabCompose.classList.remove('active');
                tabYAML.classList.add('active');
                composeControls.style.display = 'none';
                yamlControls.style.display = 'flex';
                btnDryRun.style.display = 'none';

                loadAuxiliaryYAML();
            }
        }

        function setEditorContent(content) {
            const editor = document.getElementById('modalComposeEditor');
            const preview = document.getElementById('modalComposeContent');
            if (editor) {
                editor.value = content;
                updateEditorMeta();
            }
            if (preview) {
                preview.textContent = content;
            }
        }

        function updateEditorMeta() {
            const editor = document.getElementById('modalComposeEditor');
            const meta = document.getElementById('editorMetaText');
            if (editor && meta) {
                const lines = editor.value.split('\n').length;
                const chars = editor.value.length;
                meta.textContent = 'Lines: ' + lines + ' | Chars: ' + chars;
            }
        }

        function showEditorAlert(type, message) {
            const alertEl = document.getElementById('editorAlert');
            if (!alertEl) return;
            alertEl.className = 'editor-alert ' + type;
            alertEl.textContent = message;
            alertEl.style.display = 'block';
        }

        function hideEditorAlert() {
            const alertEl = document.getElementById('editorAlert');
            if (alertEl) {
                alertEl.style.display = 'none';
                alertEl.textContent = '';
            }
        }

        function showComposeModal(stackName, initialMode) {
            currentStackName = stackName;
            const modal = document.getElementById('composeModal');
            const nameEl = document.getElementById('modalStackName');
            const copyBtn = document.getElementById('btnCopy');
            const statusPill = document.getElementById('editorStatusText');

            nameEl.textContent = stackName;
            copyBtn.textContent = '📋 Copy YAML';
            if (statusPill) statusPill.textContent = 'Ready';
            hideEditorAlert();

            modal.classList.add('open');
            switchEditorMode(initialMode || 'compose');
        }

        function fetchCompose(stackName) {
            const statusPill = document.getElementById('editorStatusText');
            if (statusPill) statusPill.textContent = 'Fetching...';

            fetch('/admin/compose?name=' + encodeURIComponent(stackName) + '&key=' + encodeURIComponent('{{.AdminKey}}'))
                .then(r => {
                    if (!r.ok) throw new Error('HTTP ' + r.status);
                    return r.json();
                })
                .then(data => {
                    const content = data.compose || '# No compose definition found.';
                    setEditorContent(content);
                    if (statusPill) statusPill.textContent = 'Loaded';
                })
                .catch(err => {
                    setEditorContent('# Failed to fetch compose definition: ' + err.message);
                    if (statusPill) statusPill.textContent = 'Error';
                    showEditorAlert('error', 'Failed to retrieve compose: ' + err.message);
                });
        }

        function loadAuxiliaryYAML() {
            const filename = (document.getElementById('auxFilename').value || 'config.yml').trim();
            const statusPill = document.getElementById('editorStatusText');
            if (statusPill) statusPill.textContent = 'Fetching ' + filename + '...';
            hideEditorAlert();

            fetch('/admin/yaml?name=' + encodeURIComponent(currentStackName) + '&filename=' + encodeURIComponent(filename) + '&key=' + encodeURIComponent('{{.AdminKey}}'))
                .then(r => {
                    if (r.status === 404) {
                        return { content: '# New configuration file: ' + filename + '\n' };
                    }
                    if (!r.ok) throw new Error('HTTP ' + r.status);
                    return r.json();
                })
                .then(data => {
                    setEditorContent(data.content || '');
                    if (statusPill) statusPill.textContent = 'Loaded ' + filename;
                })
                .catch(err => {
                    setEditorContent('# Error fetching ' + filename + ': ' + err.message);
                    if (statusPill) statusPill.textContent = 'Error';
                    showEditorAlert('error', 'Could not load YAML file: ' + err.message);
                });
        }

        function validateComposeDryRun() {
            const editor = document.getElementById('modalComposeEditor');
            const yamlContent = editor ? editor.value : '';
            const statusPill = document.getElementById('editorStatusText');
            const btn = document.getElementById('btnDryRun');

            if (!yamlContent.trim()) {
                showEditorAlert('error', 'Cannot validate an empty compose file.');
                return;
            }

            if (statusPill) statusPill.textContent = 'Validating...';
            btn.disabled = true;
            btn.textContent = '⏳ Validating...';
            hideEditorAlert();

            fetch('/api/v1/projects/' + encodeURIComponent(currentStackName) + '/compose?dry_run=true', {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json',
                    'X-Admin-Key': '{{.AdminKey}}'
                },
                body: JSON.stringify({ compose: yamlContent })
            })
            .then(async r => {
                const data = await r.json().catch(() => ({}));
                if (!r.ok) {
                    throw new Error(data.error || 'HTTP ' + r.status);
                }
                return data;
            })
            .then(data => {
                if (statusPill) statusPill.textContent = 'Valid';
                showEditorAlert('success', '✓ Validation Passed: Compose syntax is valid for stack ' + currentStackName + ' (dry run succeeded).');
            })
            .catch(err => {
                if (statusPill) statusPill.textContent = 'Invalid';
                showEditorAlert('error', '✗ Validation Failed: ' + err.message);
            })
            .finally(() => {
                btn.disabled = false;
                btn.textContent = '🧪 Validate (Dry Run)';
            });
        }

        function saveYAMLContent() {
            const editor = document.getElementById('modalComposeEditor');
            const yamlContent = editor ? editor.value : '';
            const statusPill = document.getElementById('editorStatusText');
            const btn = document.getElementById('btnSave');

            if (!yamlContent.trim()) {
                showEditorAlert('error', 'Cannot save empty content.');
                return;
            }

            btn.disabled = true;
            btn.textContent = '⏳ Saving...';
            if (statusPill) statusPill.textContent = 'Saving...';
            hideEditorAlert();

            if (currentEditorMode === 'compose') {
                const redeploy = document.getElementById('composeRedeploy').checked;
                const url = '/api/v1/projects/' + encodeURIComponent(currentStackName) + '/compose?redeploy=' + redeploy;

                fetch(url, {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-Admin-Key': '{{.AdminKey}}'
                    },
                    body: JSON.stringify({ compose: yamlContent })
                })
                .then(async r => {
                    const data = await r.json().catch(() => ({}));
                    if (!r.ok) throw new Error(data.error || 'HTTP ' + r.status);
                    return data;
                })
                .then(data => {
                    // Update cached textarea
                    const cache = document.getElementById('compose-data-' + currentStackName);
                    if (cache) cache.value = yamlContent;
                    const preview = document.getElementById('modalComposeContent');
                    if (preview) preview.textContent = yamlContent;

                    if (statusPill) statusPill.textContent = 'Saved';
                    showEditorAlert('success', '✓ Docker Compose configuration saved successfully' + (redeploy ? ' and redeployed.' : '.'));
                })
                .catch(err => {
                    if (statusPill) statusPill.textContent = 'Error';
                    showEditorAlert('error', 'Failed to save compose: ' + err.message);
                })
                .finally(() => {
                    btn.disabled = false;
                    btn.textContent = '💾 Save & Apply';
                });
            } else {
                const filename = (document.getElementById('auxFilename').value || 'config.yml').trim();
                const restart = document.getElementById('yamlRestart').checked;
                const url = '/api/v1/projects/' + encodeURIComponent(currentStackName) + '/yaml/' + encodeURIComponent(filename) + '?restart=' + restart;

                fetch(url, {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json',
                        'X-Admin-Key': '{{.AdminKey}}'
                    },
                    body: JSON.stringify({ content: yamlContent })
                })
                .then(async r => {
                    const data = await r.json().catch(() => ({}));
                    if (!r.ok) throw new Error(data.error || 'HTTP ' + r.status);
                    return data;
                })
                .then(data => {
                    if (statusPill) statusPill.textContent = 'Saved';
                    showEditorAlert('success', '✓ Auxiliary configuration file ' + filename + ' saved successfully' + (restart ? ' and stack restarted.' : '.'));
                })
                .catch(err => {
                    if (statusPill) statusPill.textContent = 'Error';
                    showEditorAlert('error', 'Failed to save YAML: ' + err.message);
                })
                .finally(() => {
                    btn.disabled = false;
                    btn.textContent = '💾 Save & Apply';
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
            const editor = document.getElementById('modalComposeEditor');
            const copyBtn = document.getElementById('btnCopy');
            const textToCopy = editor ? editor.value : '';
            if (textToCopy) {
                navigator.clipboard.writeText(textToCopy).then(() => {
                    copyBtn.textContent = '✓ Copied!';
                    setTimeout(() => {
                        copyBtn.textContent = '📋 Copy YAML';
                    }, 2000);
                }).catch(() => {
                    copyBtn.textContent = 'Error copying';
                });
            }
        }

        // Support Tab key in editor textarea for 2 spaces indentation
        document.addEventListener('DOMContentLoaded', () => {
            const editor = document.getElementById('modalComposeEditor');
            if (editor) {
                editor.addEventListener('keydown', function(e) {
                    if (e.key === 'Tab') {
                        e.preventDefault();
                        const start = this.selectionStart;
                        const end = this.selectionEnd;
                        this.value = this.value.substring(0, start) + '  ' + this.value.substring(end);
                        this.selectionStart = this.selectionEnd = start + 2;
                        updateEditorMeta();
                    }
                });
                editor.addEventListener('input', updateEditorMeta);
            }
        });

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

	loginTemplate = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sköll - Login with Discord</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;600;700&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0b0f17;
            --surface-color: #121824;
            --surface-border: #232d3f;
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --discord-blurple: #5865F2;
            --discord-hover: #4752C4;
            --accent-cyan: #38bdf8;
            --accent-red: #ef4444;
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
        .login-card {
            background: var(--surface-color);
            border: 1px solid var(--surface-border);
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6), 0 0 30px rgba(88, 101, 242, 0.1);
            border-radius: 16px;
            max-width: 440px;
            width: 100%;
            padding: 40px 36px;
            text-align: center;
        }
        .logo-symbol {
            width: 48px;
            height: 48px;
            background: linear-gradient(135deg, #0284c7, #6366f1);
            border-radius: 12px;
            display: inline-flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            font-size: 24px;
            color: white;
            box-shadow: 0 0 20px rgba(2, 132, 199, 0.4);
            margin-bottom: 20px;
        }
        h1 {
            font-size: 24px;
            font-weight: 700;
            letter-spacing: -0.5px;
            margin-bottom: 8px;
        }
        .subtitle {
            color: var(--text-secondary);
            font-size: 14px;
            margin-bottom: 28px;
            line-height: 1.5;
        }
        .btn-discord {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            gap: 12px;
            width: 100%;
            background-color: var(--discord-blurple);
            color: white;
            text-decoration: none;
            padding: 14px 24px;
            border-radius: 10px;
            font-weight: 600;
            font-size: 15px;
            transition: all 0.2s ease;
            box-shadow: 0 4px 14px rgba(88, 101, 242, 0.4);
        }
        .btn-discord:hover {
            background-color: var(--discord-hover);
            transform: translateY(-2px);
            box-shadow: 0 6px 20px rgba(88, 101, 242, 0.6);
        }
        .btn-discord svg {
            width: 22px;
            height: 22px;
            fill: currentColor;
        }
        .badge-notice {
            margin-top: 24px;
            padding: 12px 14px;
            background: rgba(30, 41, 59, 0.6);
            border: 1px solid #1e293b;
            border-radius: 8px;
            font-size: 12px;
            color: var(--text-secondary);
            line-height: 1.5;
            text-align: left;
        }
        .badge-notice strong {
            color: var(--accent-cyan);
        }
        .alert-error {
            background: rgba(239, 68, 68, 0.15);
            border: 1px solid var(--accent-red);
            color: #fca5a5;
            padding: 12px 14px;
            border-radius: 8px;
            font-size: 13px;
            margin-bottom: 20px;
            text-align: left;
        }
    </style>
</head>
<body>
    <div class="login-card">
        <div class="logo-symbol">S</div>
        <h1>Sköll Control Panel</h1>
        <p class="subtitle">Authenticate with your Discord identity to manage Docker Compose stacks, access control, and configurations.</p>

        {{if .ErrorMessage}}
        <div class="alert-error">
            {{.ErrorMessage}}
        </div>
        {{end}}

        <a href="/auth/login" class="btn-discord" id="discord-login-btn">
            <svg viewBox="0 0 127.14 96.36">
                <path d="M107.7,8.07A105.15,105.15,0,0,0,81.47,0a72.06,72.06,0,0,0-3.36,6.83A97.68,97.68,0,0,0,49,6.83,72.37,72.37,0,0,0,45.64,0,105.89,105.89,0,0,0,19.39,8.09C2.79,32.65-1.71,56.6.54,80.21h0A105.73,105.73,0,0,0,32.71,96.36,77.7,77.7,0,0,0,39.6,85.25a68.42,68.42,0,0,1-10.85-5.18c.91-.66,1.8-1.34,2.66-2a75.57,75.57,0,0,0,64.32,0c.87.71,1.76,1.39,2.66,2a68.68,68.68,0,0,1-10.87,5.19,77,77,0,0,0,6.89,11.1A105.25,105.25,0,0,0,126.6,80.22h0C129.24,52.84,122.09,29.11,107.7,8.07ZM42.45,65.69C36.18,65.69,31,60,31,53s5-12.74,11.43-12.74S54,45.91,53.89,53,48.84,65.69,42.45,65.69Zm42.24,0C78.41,65.69,73.25,60,73.25,53s5-12.74,11.44-12.74S96.23,45.91,96.12,53,91.08,65.69,84.69,65.69Z"/>
            </svg>
            <span>Login with Discord</span>
        </a>

        <div class="badge-notice">
            <strong>Access Requirements:</strong> Operator or Admin role required. The first linked Discord user is automatically granted Administrator privileges.
        </div>
    </div>
</body>
</html>`))

	adminLoginTemplate = loginTemplate

	forbiddenTemplate = template.Must(template.New("forbidden").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sköll - 403 Forbidden</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;600;700&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0b0f17;
            --surface-color: #121824;
            --surface-border: #ef4444;
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent-cyan: #38bdf8;
            --accent-red: #ef4444;
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
            border: 1px solid var(--surface-border);
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6), 0 0 30px rgba(239, 68, 68, 0.15);
            border-radius: 16px;
            max-width: 480px;
            width: 100%;
            padding: 40px 36px;
            text-align: center;
        }
        .icon {
            font-size: 42px;
            margin-bottom: 16px;
        }
        h1 {
            font-size: 24px;
            font-weight: 700;
            color: var(--accent-red);
            margin-bottom: 12px;
        }
        p {
            color: var(--text-secondary);
            font-size: 14px;
            line-height: 1.6;
            margin-bottom: 24px;
        }
        .user-box {
            background: #1e293b;
            border: 1px solid #334155;
            border-radius: 8px;
            padding: 12px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
            margin-bottom: 24px;
            color: var(--accent-cyan);
        }
        .btn-group {
            display: flex;
            gap: 12px;
            justify-content: center;
        }
        .btn {
            display: inline-block;
            background: #232d3f;
            color: white;
            text-decoration: none;
            padding: 10px 20px;
            border-radius: 8px;
            font-weight: 600;
            font-size: 13px;
            transition: all 0.2s;
        }
        .btn:hover { background: #334155; }
        .btn-primary { background: #5865F2; }
        .btn-primary:hover { background: #4752C4; }
    </style>
</head>
<body>
    <div class="card">
        <div class="icon">🚫</div>
        <h1>403 Forbidden</h1>
        <p>{{.ErrorMessage}}</p>
        {{if .Username}}
        <div class="user-box">
            Account: <strong>{{.Username}}</strong> &bull; Role: <span style="color: #f59e0b;">{{.Role}}</span>
        </div>
        {{end}}
        <div class="btn-group">
            <a href="/auth/logout" class="btn">Log Out</a>
            <a href="/auth/login" class="btn btn-primary">Switch Discord Account</a>
        </div>
    </div>
</body>
</html>`))
)
