import 'dart:async';
import 'dart:io';

const int defaultDevServerPort = 34987;
const Duration coreStartupTimeout = Duration(minutes: 3);
const String testWoxDataDirEnv = 'WOX_TEST_DATA_DIR';
const String testUserDataDirEnv = 'WOX_TEST_USER_DIR';
const String testServerPortEnv = 'WOX_TEST_SERVER_PORT';
const String testDisableTelemetryEnv = 'WOX_TEST_DISABLE_TELEMETRY';

Future<void> main(List<String> args) async {
  if (args.isEmpty || args.first == 'help' || args.first == '--help') {
    _printHelp();
    exitCode = 0;
    return;
  }

  switch (args.first) {
    case 'smoke':
      exitCode = await _runSmoke();
      return;
    default:
      stderr.writeln('Unknown command: ${args.first}');
      _printHelp();
      exitCode = 64;
  }
}

void _printHelp() {
  stdout.writeln('Usage: dart run bin/run.dart <command>');
  stdout.writeln('');
  stdout.writeln('Commands:');
  stdout.writeln('  smoke    Run the desktop smoke E2E flow');
}

Future<int> _runSmoke() async {
  final packageRoot = _resolvePackageRoot();
  final repoRoot = packageRoot.parent;
  final artifactsDir = await _createArtifactsDir(packageRoot);
  final woxDataDir = Directory('${artifactsDir.path}${Platform.pathSeparator}wox-data');
  final userDataDir = Directory('${artifactsDir.path}${Platform.pathSeparator}user-data');
  await woxDataDir.create(recursive: true);
  await userDataDir.create(recursive: true);
  final serverPort = await _reserveServerPort();

  final environment = Map<String, String>.from(Platform.environment);
  environment[testWoxDataDirEnv] = woxDataDir.path;
  environment[testUserDataDirEnv] = userDataDir.path;
  environment[testServerPortEnv] = '$serverPort';
  environment[testDisableTelemetryEnv] = 'true';

  stdout.writeln('Artifacts: ${artifactsDir.path}');
  stdout.writeln('Wox data dir: ${woxDataDir.path}');
  stdout.writeln('Wox user dir: ${userDataDir.path}');
  stdout.writeln('Server port: $serverPort');

  final coreLog = File('${artifactsDir.path}${Platform.pathSeparator}core.log');
  final testLog = File('${artifactsDir.path}${Platform.pathSeparator}flutter_test.log');

  Process? coreProcess;
  try {
    coreProcess = await _startCommand('go', ['run', '.'], workingDirectory: '${repoRoot.path}${Platform.pathSeparator}wox.core', environment: environment);
    await _pipeProcessOutput(coreProcess, coreLog, '[core]');

    final ready = await _waitForPingReady(serverPort: serverPort, timeout: coreStartupTimeout);
    if (!ready) {
      stderr.writeln('wox.core did not become ready on port $serverPort.');
      return 1;
    }

    final flutterBuildConflict = await _findRunningFlutterBuildExecutable(repoRoot);
    if (flutterBuildConflict != null) {
      stderr.writeln('Close the running Flutter development UI before smoke test: $flutterBuildConflict');
      return 1;
    }

    final flutterProcess = await _startCommand(
      'flutter',
      ['test', '--dart-define=$testServerPortEnv=$serverPort', 'integration_test/launcher_smoke_test.dart'],
      workingDirectory: '${repoRoot.path}${Platform.pathSeparator}wox.ui.flutter${Platform.pathSeparator}wox',
      environment: environment,
    );
    await _pipeProcessOutput(flutterProcess, testLog, '[flutter-test]');
    return await flutterProcess.exitCode;
  } finally {
    if (coreProcess != null) {
      await _terminateProcess(coreProcess);
    }
  }
}

Future<Process> _startCommand(String command, List<String> arguments, {required String workingDirectory, required Map<String, String> environment}) {
  if (Platform.isWindows) {
    return Process.start('cmd.exe', ['/c', command, ...arguments], workingDirectory: workingDirectory, environment: environment);
  }

  return Process.start(command, arguments, workingDirectory: workingDirectory, environment: environment);
}

Future<String?> _findRunningFlutterBuildExecutable(Directory repoRoot) async {
  if (!Platform.isWindows) {
    return null;
  }

  final targetPath =
      '${repoRoot.path}${Platform.pathSeparator}wox.ui.flutter${Platform.pathSeparator}wox${Platform.pathSeparator}build${Platform.pathSeparator}windows${Platform.pathSeparator}x64${Platform.pathSeparator}runner${Platform.pathSeparator}Debug${Platform.pathSeparator}wox-ui.exe';
  final result = await Process.run('powershell.exe', ['-NoProfile', '-Command', 'Get-Process wox-ui -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Path']);

  final runningPaths = result.stdout.toString().split(RegExp(r'[\r\n]+')).map((line) => line.trim()).where((line) => line.isNotEmpty);
  for (final path in runningPaths) {
    if (path.toLowerCase() == targetPath.toLowerCase()) {
      return path;
    }
  }

  return null;
}

Directory _resolvePackageRoot() {
  final current = Directory.current;
  if (File('${current.path}${Platform.pathSeparator}pubspec.yaml').existsSync() && current.path.endsWith('${Platform.pathSeparator}wox.test')) {
    return current;
  }

  final nested = Directory('${current.path}${Platform.pathSeparator}wox.test');
  if (File('${nested.path}${Platform.pathSeparator}pubspec.yaml').existsSync()) {
    return nested;
  }

  throw StateError('Unable to locate wox.test package root from ${current.path}. Run this command from wox.test or the repository root.');
}

Future<Directory> _createArtifactsDir(Directory packageRoot) async {
  final timestamp = _formatLocalArtifactsTimestamp(DateTime.now());
  final dir = Directory('${packageRoot.path}${Platform.pathSeparator}artifacts${Platform.pathSeparator}$timestamp');
  await dir.create(recursive: true);
  return dir;
}

String _formatLocalArtifactsTimestamp(DateTime value) {
  String twoDigits(int part) => part.toString().padLeft(2, '0');

  return '${value.year}-${twoDigits(value.month)}-${twoDigits(value.day)} '
      '${twoDigits(value.hour)}-${twoDigits(value.minute)}-${twoDigits(value.second)}';
}

Future<int> _reserveServerPort() async {
  try {
    final preferredSocket = await ServerSocket.bind(InternetAddress.loopbackIPv4, defaultDevServerPort);
    final port = preferredSocket.port;
    await preferredSocket.close();
    return port;
  } on SocketException {
    final fallbackSocket = await ServerSocket.bind(InternetAddress.loopbackIPv4, 0);
    final port = fallbackSocket.port;
    await fallbackSocket.close();
    return port;
  }
}

Future<void> _pipeProcessOutput(Process process, File outputFile, String prefix) async {
  final sink = outputFile.openWrite(mode: FileMode.writeOnlyAppend);

  void forward(List<int> data, IOSink target) {
    sink.add(data);
    target.add(data);
  }

  process.stdout.listen((data) => forward(data, stdout));
  process.stderr.listen((data) => forward(data, stderr));

  unawaited(
    process.exitCode.whenComplete(() async {
      await sink.flush();
      await sink.close();
    }),
  );
  stdout.writeln('$prefix output -> ${outputFile.path}');
}

Future<bool> _waitForPingReady({required int serverPort, required Duration timeout}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    if (await _isPingReady(serverPort)) {
      return true;
    }
    await Future<void>.delayed(const Duration(milliseconds: 500));
  }
  return false;
}

Future<bool> _isPingReady(int serverPort) async {
  final client = HttpClient();
  try {
    final request = await client.getUrl(Uri.parse('http://127.0.0.1:$serverPort/ping'));
    final response = await request.close();
    await response.drain<void>();
    return response.statusCode == 200;
  } catch (_) {
    return false;
  } finally {
    client.close(force: true);
  }
}

Future<void> _terminateProcess(Process process) async {
  if (Platform.isWindows) {
    await Process.run('taskkill', ['/PID', '${process.pid}', '/T', '/F']);
    return;
  }

  process.kill();
  try {
    await process.exitCode.timeout(const Duration(seconds: 5));
  } on TimeoutException {
    process.kill(ProcessSignal.sigkill);
    await process.exitCode;
  }
}
