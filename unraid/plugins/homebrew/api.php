<?php
header('Content-Type: application/json; charset=utf-8');

$configDir = '/boot/config/plugins/homebrew';
$configFile = $configDir . '/homebrew.conf';
$serviceScript = '/etc/rc.d/rc.homebrew';

function json_out($success, $data = [], $error = '') {
    echo json_encode([
        'success' => $success,
        'data' => $data,
        'error' => $error,
    ], JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    exit;
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

function run_script($action, $args = []) {
    global $serviceScript;
    $cmd = '/bin/bash ' . escapeshellarg($serviceScript) . ' ' . escapeshellarg($action);
    foreach ($args as $a) {
        $cmd .= ' ' . escapeshellarg($a);
    }
    $cmd .= ' 2>&1';
    return shell_exec($cmd) ?? '';
}

function safe_conf_value($value) {
    $value = str_replace(["\n", "\r"], ['', ''], $value);
    $value = str_replace(['$', '`'], ['\$', '\`'], $value);
    $value = str_replace('"', '', $value);
    return $value;
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
    $output = run_script('status');
    $status = parse_kv_lines($output);
    json_out(true, $status);
}

if (in_array($action, ['install', 'setup', 'update', 'upgrade'], true)) {
    if ($method !== 'POST') json_out(false, [], 'method not allowed');
    $output = run_script($action);
    $status = parse_kv_lines(run_script('status'));
    json_out(true, ['output' => $output, 'status' => $status]);
}

if ($action === 'remove') {
    if ($method !== 'POST') json_out(false, [], 'method not allowed');
    $output = run_script('remove');
    json_out(true, ['output' => $output]);
}

if ($action === 'list_packages') {
    $output = run_script('list');
    $packages = array_values(array_filter(preg_split('/\s+/', trim($output))));
    json_out(true, ['packages' => $packages]);
}

if ($action === 'list_casks') {
    $output = run_script('list_cask');
    $casks = array_values(array_filter(preg_split('/\s+/', trim($output))));
    json_out(true, ['casks' => $casks]);
}

if ($action === 'outdated') {
    $output = run_script('outdated');
    json_out(true, ['outdated' => $output]);
}

if ($action === 'search') {
    $q = trim($_GET['q'] ?? $_POST['q'] ?? '');
    if (empty($q)) json_out(false, [], 'search query required');
    $output = run_script('search', [$q]);
    json_out(true, ['output' => $output, 'query' => $q]);
}

if ($action === 'package_install' && $method === 'POST') {
    $formula = trim($_POST['formula'] ?? '');
    if (empty($formula)) json_out(false, [], 'formula required');
    $output = run_script('install_pkg', [$formula]);
    json_out(true, ['output' => $output, 'formula' => $formula]);
}

if ($action === 'package_uninstall' && $method === 'POST') {
    $formula = trim($_POST['formula'] ?? '');
    if (empty($formula)) json_out(false, [], 'formula required');
    $output = run_script('uninstall_pkg', [$formula]);
    json_out(true, ['output' => $output, 'formula' => $formula]);
}

if ($action === 'package_info') {
    $formula = trim($_GET['formula'] ?? $_POST['formula'] ?? '');
    if (empty($formula)) json_out(false, [], 'formula required');
    $output = run_script('info', [$formula]);
    json_out(true, ['info' => $output, 'formula' => $formula]);
}

if ($action === 'save_config') {
    if ($method !== 'POST') json_out(false, [], 'method not allowed');
    $body = $_POST;

    $fields = ['brew_storage', 'autostart', 'shell_integration', 'gcc_autoinstall'];

    $content = "# Homebrew plugin configuration\n";
    foreach ($fields as $f) {
        $val = $body[$f] ?? '';
        if ($f === 'autostart' || $f === 'gcc_autoinstall') {
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

json_out(false, [], 'unknown action: ' . $action);
