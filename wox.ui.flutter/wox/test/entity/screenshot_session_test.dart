import 'package:flutter_test/flutter_test.dart';
import 'package:wox/entity/screenshot_session.dart';

void main() {
  group('ScreenshotSession entity parsing', () {
    test('DisplaySnapshot.fromJson accepts json-like nested maps from method channels', () {
      final json = <String, dynamic>{
        'displayId': 'display-a',
        'logicalBounds': <Object?, Object?>{'x': 0, 'y': 0, 'width': 1920, 'height': 1080},
        'pixelBounds': <Object?, Object?>{'x': 0, 'y': 0, 'width': 1920, 'height': 1080},
        'scale': 1,
        'rotation': 0,
        'imageBytesBase64': '',
      };

      expect(() => DisplaySnapshot.fromJson(json), returnsNormally, reason: 'MethodChannel payloads use json-like maps whose nested values are typed as Map<Object?, Object?>.');
    });
  });
}
