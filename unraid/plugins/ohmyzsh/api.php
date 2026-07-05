<?php
header('Content-Type: application/json; charset=utf-8');

$configDir = '/boot/config/plugins/ohmyzsh';
$configFile = $configDir . '/ohmyzsh.conf';
$serviceScript = '/etc/rc.d/rc.ohmyzsh';

function json_out($success, $data = [], $error = '') {
    echo json_encode([
        'success' => $success,
        'data' => $data,
        'error' => $error,
    ], JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES);
    exit;
}

function get_config() {
    global $configFile;
    $defaults = [
        'autostart' => 'yes',
        'zsh_theme' => 'robbyrussell',
        'zsh_plugins' => 'git',
        'custom_aliases' => '',
    ];
    if (!file_exists($configFile)) return $defaults;
    $lines = file($configFile, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES);
    foreach ($lines as $line) {
        if (strpos($line, '=') === false || $line[0] === '#') continue;
        [$k, $v] = explode('=', $line, 2);
        $k = strtolower(trim($k));
        $v = trim($v, " \t\n\r\0\x0B\"'");
        if (array_key_exists($k, $defaults)) $defaults[$k] = $v;
    }
    return $defaults;
}

$action = $_GET['action'] ?? $_POST['action'] ?? 'status';

if ($action === 'status') {
    $installed = is_dir('/boot/config/plugins/ohmyzsh/oh-my-zsh') ? '1' : '0';
    $cfg = get_config();
    $shell = trim(shell_exec('grep "^root:" /etc/passwd | cut -d: -f7') ?? '');
    $data = [
        'installed' => $installed,
        'version' => $installed ? trim(shell_exec("ZSH=/boot/config/plugins/ohmyzsh/oh-my-zsh zsh -c 'echo \$ZSH_VERSION' 2>/dev/null") ?? '-') : '-',
        'theme' => $cfg['zsh_theme'],
        'plugins' => $cfg['zsh_plugins'],
        'shell' => $shell === '/bin/zsh' ? 'zsh (active)' : ($shell ? basename($shell) : 'unknown'),
        'config' => file_exists('/root/.zshrc') ? '/root/.zshrc' : 'not set',
    ];
    json_out(true, $data);
}

if ($action === 'get_config') {
    json_out(true, get_config());
}

if ($action === 'save_config') {
    $body = $_POST;
    $content = "# oh-my-zsh plugin configuration\n";
    $fields = ['autostart', 'zsh_theme', 'zsh_plugins', 'custom_aliases'];
    foreach ($fields as $f) {
        $val = $body[$f] ?? '';
        if ($f === 'autostart') {
            $val = in_array($val, ['yes', 'no']) ? $val : 'yes';
        }
        $val = str_replace(["\n", "\r"], ['\\n', ''], $val);
        $val = str_replace(['$', '`', '"'], ['\$', '\`', ''], $val);
        $content .= strtoupper($f) . '="' . $val . "\"\n";
    }
    $tmp = $configFile . '.tmp';
    if (file_put_contents($tmp, $content) === false) json_out(false, [], 'write failed');
    if (!rename($tmp, $configFile)) { @unlink($tmp); json_out(false, [], 'rename failed'); }
    json_out(true, ['saved' => true]);
}

if (in_array($action, ['install', 'update', 'reset'], true)) {
    if ($_SERVER['REQUEST_METHOD'] !== 'POST') json_out(false, [], 'method not allowed');
    $output = shell_exec('/bin/bash ' . escapeshellarg($serviceScript) . ' ' . escapeshellarg($action) . ' 2>&1');
    json_out(true, ['output' => $output ?? '']);
}

json_out(false, [], 'unknown action: ' . $action);
