import 'dart:async';
import 'dart:convert';
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
      final testName = args.length > 1 ? args.sublist(1).join(' ').trim() : null;
      // Use exit() explicitly because Future.any leaves dangling futures
      // (10-min timeout timer, file-polling loop) that keep the event loop alive.
      exit(await _runSmoke(testName: testName?.isEmpty == true ? null : testName));
    default:
      stderr.writeln('Unknown command: ${args.first}');
      _printHelp();
      exitCode = 64;
  }
}

void _printHelp() {
  stdout.writeln('Usage: dart run bin/run.dart <command> [arguments]');
  stdout.writeln('');
  stdout.writeln('Commands:');
  stdout.writeln('  smoke [test name]    Run the desktop smoke E2E flow');
}

Future<int> _runSmoke({String? testName}) async {
  final packageRoot = _resolvePackageRoot();
  final repoRoot = packageRoot.parent;
  final artifactsDir = await _createArtifactsDir(packageRoot);
  final coreBinary = File('${artifactsDir.path}${Platform.pathSeparator}wox-core-smoke${Platform.isWindows ? '.exe' : ''}');
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
  if (testName != null) {
    stdout.writeln('Test filter: $testName');
  }

  final coreLog = File('${artifactsDir.path}${Platform.pathSeparator}core.log');
  final testLog = File('${artifactsDir.path}${Platform.pathSeparator}flutter_test.log');

  Process? coreProcess;
  try {
    stdout.writeln('Building core smoke binary...');
    await _buildCoreBinary(workingDirectory: '${repoRoot.path}${Platform.pathSeparator}wox.core', outputPath: coreBinary.path, environment: environment);

    coreProcess = await _startCommand(coreBinary.path, [], workingDirectory: '${repoRoot.path}${Platform.pathSeparator}wox.core', environment: environment);
    _pipeProcessOutput(coreProcess, coreLog, '[core]');

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

    final flutterArgs = <String>[
      'test',
      '--dart-define=$testServerPortEnv=$serverPort',
      if (testName != null) ...['--plain-name', testName],
      'integration_test/launcher_smoke_test.dart',
    ];

    final flutterProcess = await _startCommand(
      'flutter',
      flutterArgs,
      workingDirectory: '${repoRoot.path}${Platform.pathSeparator}wox.ui.flutter${Platform.pathSeparator}wox',
      environment: environment,
    );
    const completionMarkers = ['All tests passed!', 'Some tests failed.'];
    final testsFinished = _pipeProcessOutput(flutterProcess, testLog, '[flutter-test]', completionMarkers: completionMarkers);
    final completionDetected = Future.any<bool>([testsFinished.future, _waitForCompletionMarkerInFile(testLog, completionMarkers)]);

    // On macOS the integration test app may not exit on its own after all
    // tests finish.  Wait for the completion marker, then give the process a
    // short grace period before terminating it.
    int flutterExitCode;

    // First, wait for the process to exit naturally OR for tests to finish.
    // Also add a hard timeout so CI never hangs indefinitely.
    final processExit = flutterProcess.exitCode;
    final result = await Future.any([
      processExit.then((code) => ('exited', code)),
      completionDetected.then((passed) => ('finished', passed ? 0 : 1)),
      Future.delayed(const Duration(minutes: 10), () => ('timeout', 1)),
    ]);

    if (result.$1 == 'exited') {
      flutterExitCode = result.$2;
    } else {
      // Tests finished (or hard timeout reached) but process is still running.
      // Give it a short grace period to exit cleanly, then force-terminate.
      flutterExitCode = result.$2;
      if (result.$1 == 'timeout') {
        stderr.writeln('flutter test process hit hard timeout (10 min), terminating...');
      }
      try {
        await processExit.timeout(const Duration(seconds: 10));
      } on TimeoutException {
        stdout.writeln('flutter test process did not exit after tests completed, terminating...');
        await _terminateProcess(flutterProcess);
      }
    }
    return flutterExitCode;
  } finally {
    if (coreProcess != null) {
      await _terminateProcess(coreProcess);
    }
  }
}

Future<void> _buildCoreBinary({required String workingDirectory, required String outputPath, required Map<String, String> environment}) async {
  final buildArgs = ['build', '-o', outputPath, '.'];
  final buildProcess = await _startCommand('go', buildArgs, workingDirectory: workingDirectory, environment: environment);

  final stdoutBuffer = StringBuffer();
  final stderrBuffer = StringBuffer();
  buildProcess.stdout.transform(utf8.decoder).listen(stdoutBuffer.write);
  buildProcess.stderr.transform(utf8.decoder).listen(stderrBuffer.write);

  final exitCode = await buildProcess.exitCode;
  if (exitCode != 0) {
    final message = [
      'Failed to build smoke core binary (exit $exitCode).',
      if (stdoutBuffer.isNotEmpty) stdoutBuffer.toString().trimRight(),
      if (stderrBuffer.isNotEmpty) stderrBuffer.toString().trimRight(),
    ].where((part) => part.isNotEmpty).join('\n');
    throw ProcessException('go', buildArgs, message, exitCode);
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

/// Pipes process stdout/stderr to [outputFile] and the console.
///
/// When [completionMarkers] is provided, the returned [Completer] completes
/// as soon as any marker string appears in stdout or stderr.  If
/// [completionMarkers] is null the completer never completes on its own.
Completer<bool> _pipeProcessOutput(Process process, File outputFile, String prefix, {List<String>? completionMarkers}) {
  final testsFinished = Completer<bool>();
  final sink = outputFile.openWrite(mode: FileMode.writeOnlyAppend);
  // Buffer recent output to detect markers that span chunk boundaries.
  final _recentOutput = StringBuffer();

  void _checkMarkers(String text) {
    if (completionMarkers == null || testsFinished.isCompleted) return;
    _recentOutput.write(text);
    // Keep only the last 4 KB to bound memory usage.
    if (_recentOutput.length > 4096) {
      final s = _recentOutput.toString();
      _recentOutput.clear();
      _recentOutput.write(s.substring(s.length - 2048));
    }
    final buffer = _recentOutput.toString();
    if (completionMarkers.any((m) => buffer.contains(m))) {
      testsFinished.complete(buffer.contains('All tests passed!'));
    }
  }

  void forward(List<int> data, IOSink target) {
    sink.add(data);
    target.add(data);
    _checkMarkers(utf8.decode(data, allowMalformed: true));
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
  return testsFinished;
}

Future<bool> _waitForCompletionMarkerInFile(File outputFile, List<String> completionMarkers) async {
  var previousLength = -1;

  while (true) {
    try {
      if (await outputFile.exists()) {
        final text = await outputFile.readAsString();
        if (text.length != previousLength) {
          previousLength = text.length;
          if (text.contains('All tests passed!')) {
            return true;
          }
          if (text.contains('Some tests failed.')) {
            return false;
          }
          for (final marker in completionMarkers) {
            if (text.contains(marker)) {
              return marker == 'All tests passed!';
            }
          }
        }
      }
    } catch (_) {
      // Ignore transient file-read failures while the log file is still being written.
    }

    await Future<void>.delayed(const Duration(milliseconds: 250));
  }
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
