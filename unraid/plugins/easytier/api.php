<?php
header('Content-Type: application/json; charset=utf-8');

$configDir = '/boot/config/plugins/easytier';
$configFile = $configDir . '/easytier.conf';
$serviceScript = '/etc/rc.d/rc.easytier';

function json_out($success, $data = [], $error = '') {
    echo json_encode([
        'success' => $success,
        'data' => $data,
        'error' => $error,
    ], JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    exit;
}

function read_body_json() {
    $raw = file_get_contents('php://input');
    if ($raw) {
        $decoded = json_decode($raw, true);
        if (is_array($decoded)) return $decoded;
    }
    return $_POST;
}

function parse_kv_lines($text) {
    $result = [];
    $lines = preg_split('/\r\n|\r|\n/', $text);
    foreach ($lines as $line) {
        if (strpos($line, '=') === false) continue;
        [$k, $v] = explode('=', $line, 2);
        $result[trim($k)] = trim($v, " \t\n\r\0\x0B\"");
    }
    return $result;
}

function run_service_cmd($action) {
    global $serviceScript;
    $cmd = '/bin/bash ' . escapeshellarg($serviceScript) . ' ' . escapeshellarg($action) . ' 2>&1';
    return shell_exec($cmd) ?? '';
}

function safe_conf_value($value) {
    $value = str_replace(["\n", "\r"], ['', ''], $value);
    $value = str_replace(['$', '`'], ['\$', '\`'], $value);
    $value = str_replace('"', '', $value);
    return $value;
}

function cli_json_args(...$args) {
    $parts = ['/usr/bin/easytier-cli', '-o', 'json'];
    foreach ($args as $a) $parts[] = escapeshellarg((string)$a);
    $cmd = implode(' ', $parts) . ' 2>&1';
    $output = shell_exec($cmd) ?? '';
    $decoded = json_decode($output, true);
    if (json_last_error() !== JSON_ERROR_NONE) {
        return ['_raw' => $output, '_error' => json_last_error_msg()];
    }
    return $decoded;
}

function cli_run_args(...$args) {
    if (empty($args)) return '';
    $parts = ['/usr/bin/easytier-cli'];
    foreach ($args as $a) $parts[] = escapeshellarg((string)$a);
    $cmd = implode(' ', $parts) . ' 2>&1';
    return shell_exec($cmd) ?? '';
}

if (!is_dir($configDir)) {
    @mkdir($configDir, 0777, true);
}

$action = $_GET['action'] ?? $_POST['action'] ?? 'status';
$method = $_SERVER['REQUEST_METHOD'] ?? 'GET';

if (!file_exists($serviceScript)) {
    json_out(false, [], 'service script not found: ' . $serviceScript);
}

if ($action === 'status') {
    $output = run_service_cmd('status');
    $status = parse_kv_lines($output);
    json_out(true, $status);
}

if ($action === 'logs') {
    $status = parse_kv_lines(run_service_cmd('status'));
    $logFile = $status['log_file'] ?? ($configDir . '/easytier.log');
    if (!file_exists($logFile)) {
        json_out(true, ['logs' => '']);
    }
    $tail = shell_exec('/usr/bin/tail -n 200 ' . escapeshellarg($logFile) . ' 2>&1');
    json_out(true, ['logs' => $tail ?? '']);
}

if (in_array($action, ['start', 'stop', 'restart'], true)) {
    if ($method !== 'POST') json_out(false, [], 'method not allowed');
    $output = run_service_cmd($action);
    $status = parse_kv_lines(run_service_cmd('status'));
    json_out(true, ['output' => $output, 'status' => $status]);
}

if ($action === 'save_config') {
    if ($method !== 'POST') json_out(false, [], 'method not allowed');
    $body = $_POST;

    $fields = [
        'network_name', 'network_secret', 'dhcp', 'virtual_ipv4',
        'hostname', 'peer_urls', 'listener_urls', 'proxy_cidrs', 'autostart'
    ];

    $content = "# EasyTier plugin configuration\n";
    foreach ($fields as $f) {
        $val = $body[$f] ?? '';
        if ($f === 'dhcp' || $f === 'autostart') {
            $val = strtolower(trim((string)($body[$f] ?? 'yes')));
            if (!in_array($val, ['yes', 'no'], true)) $val = 'yes';
        }
        $content .= strtoupper($f) . '="' . safe_conf_value($val) . "\"\n";
    }

    $tmpFile = $configFile . '.tmp';
    if (file_put_contents($tmpFile, $content) === false) {
        json_out(false, [], 'failed to write temp config');
    }
    if (!rename($tmpFile, $configFile)) {
        @unlink($tmpFile);
        json_out(false, [], 'failed to update config');
    }
    json_out(true, ['saved' => true]);
}

// --- easytier-cli data actions ---

if ($action === 'node_info') {
    $data = cli_json_args('node', 'info');
    json_out(true, $data);
}

if ($action === 'peers') {
    $data = cli_json_args('peer', 'list');
    json_out(true, $data);
}

if ($action === 'routes') {
    $data = cli_json_args('route', 'list');
    json_out(true, $data);
}

if ($action === 'stats') {
    $data = cli_json_args('stats', 'show');
    json_out(true, $data);
}

// --- Runtime data modification ---

// Connector management
if ($action === 'connector_list') {
    $data = cli_json_args('connector', 'list');
    json_out(true, $data);
}

if ($action === 'connector_add' && $method === 'POST') {
    $body = read_body_json();
    $url = trim($body['url'] ?? '');
    if (empty($url)) json_out(false, [], 'url is required');
    $output = cli_run_args('connector', 'add', $url);
    json_out(true, ['output' => $output]);
}

if ($action === 'connector_remove' && $method === 'POST') {
    $body = read_body_json();
    $url = trim($body['url'] ?? '');
    if (empty($url)) json_out(false, [], 'url is required');
    $output = cli_run_args('connector', 'remove', $url);
    json_out(true, ['output' => $output]);
}

// Port forwarding
if ($action === 'portforward_list') {
    $data = cli_json_args('port-forward', 'list');
    json_out(true, $data);
}

if ($action === 'portforward_add' && $method === 'POST') {
    $body = read_body_json();
    $proto = trim($body['proto'] ?? 'tcp');
    $bind = trim($body['bind'] ?? '');
    $dst = trim($body['dst'] ?? '');
    if (empty($bind) || empty($dst)) json_out(false, [], 'bind and dst are required');
    $output = cli_run_args('port-forward', 'add', $proto, $bind, $dst);
    json_out(true, ['output' => $output]);
}

if ($action === 'portforward_remove' && $method === 'POST') {
    $body = read_body_json();
    $proto = trim($body['proto'] ?? 'tcp');
    $bind = trim($body['bind'] ?? '');
    $dst = trim($body['dst'] ?? '');
    if (empty($bind)) json_out(false, [], 'bind is required');
    $args = ['port-forward', 'remove', $proto, $bind];
    if (!empty($dst)) $args[] = $dst;
    $output = cli_run_args(...$args);
    json_out(true, ['output' => $output]);
}

// Credential management
if ($action === 'credential_list') {
    $data = cli_json_args('credential', 'list');
    json_out(true, $data);
}

if ($action === 'credential_generate' && $method === 'POST') {
    $body = read_body_json();
    $ttl = max(60, min(31536000, (int)($body['ttl'] ?? 3600)));
    $output = cli_run_args('credential', 'generate', '--ttl', (string)$ttl);
    $cred = '';
    if (preg_match('/([a-f0-9-]+)/i', $output, $m)) {
        $cred = $m[1];
    }
    json_out(true, ['output' => $output, 'credential' => $cred]);
}

if ($action === 'credential_revoke' && $method === 'POST') {
    $body = read_body_json();
    $id = trim($body['id'] ?? '');
    if (empty($id)) json_out(false, [], 'credential id is required');
    $output = cli_run_args('credential', 'revoke', $id);
    json_out(true, ['output' => $output]);
}

// Logger
if ($action === 'logger_get') {
    $data = cli_json_args('logger', 'get');
    json_out(true, $data);
}

if ($action === 'logger_set' && $method === 'POST') {
    $body = read_body_json();
    $level = trim($body['level'] ?? 'info');
    $valid = ['disabled', 'error', 'warning', 'info', 'debug', 'trace'];
    if (!in_array($level, $valid, true)) json_out(false, [], 'invalid level: ' . $level);
    $output = cli_run_args('logger', 'set', $level);
    json_out(true, ['output' => $output]);
}

json_out(false, [], 'unknown action: ' . $action);
