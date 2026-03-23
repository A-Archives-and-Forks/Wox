import 'package:integration_test/integration_test.dart';

import 'launcher_core_smoke_test.dart';
import 'launcher_key_functionality_smoke_test.dart';
import 'launcher_plugin_smoke_test.dart';
import 'launcher_system_plugin_smoke_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  registerLauncherCoreSmokeTests();
  registerLauncherKeyFunctionalitySmokeTests();
  registerLauncherPluginSmokeTests();
  registerSystemPluginSmokeTests();
}
