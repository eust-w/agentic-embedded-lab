//go:build darwin

#import <AppKit/AppKit.h>

@interface SPUStandardUpdaterController : NSObject
- (instancetype)initWithStartingUpdater:(BOOL)startingUpdater
                         updaterDelegate:(id)updaterDelegate
                      userDriverDelegate:(id)userDriverDelegate;
@end

static SPUStandardUpdaterController *aetherUpdaterController;

bool AetherSparkleAvailable(void) {
    NSString *frameworkPath = [[[NSBundle mainBundle] privateFrameworksPath]
        stringByAppendingPathComponent:@"Sparkle.framework"];
    return [[NSFileManager defaultManager] fileExistsAtPath:frameworkPath];
}

bool AetherStartSparkle(void) {
    if (aetherUpdaterController != nil) {
        return true;
    }
    if (!AetherSparkleAvailable()) {
        return false;
    }
    NSDictionary *info = [[NSBundle mainBundle] infoDictionary];
    NSString *feedURL = info[@"SUFeedURL"];
    NSString *publicKey = info[@"SUPublicEDKey"];
    if (feedURL.length == 0 || publicKey.length == 0) {
        return false;
    }
    NSString *frameworkPath = [[[NSBundle mainBundle] privateFrameworksPath]
        stringByAppendingPathComponent:@"Sparkle.framework"];
    NSBundle *framework = [NSBundle bundleWithPath:frameworkPath];
    if (![framework load]) {
        return false;
    }
    Class updaterClass = NSClassFromString(@"SPUStandardUpdaterController");
    if (updaterClass == Nil) {
        return false;
    }
    aetherUpdaterController = [[updaterClass alloc]
        initWithStartingUpdater:YES updaterDelegate:nil userDriverDelegate:nil];
    return aetherUpdaterController != nil;
}
