//go:build darwin

#import <Cocoa/Cocoa.h>

extern void goReopenApplication(void);

@interface EXReopenHandler : NSObject
@end

@implementation EXReopenHandler
- (void)handleReopen:(NSAppleEventDescriptor *)event
      withReplyEvent:(NSAppleEventDescriptor *)reply {
    goReopenApplication();
}
@end

static EXReopenHandler *reopenHandler;

// registerReopenHandler installs a handler for the kAEReopenApplication
// ("rapp") Apple event. It runs on the main queue, which GLFW's event loop
// services, because AppKit expects Apple event handlers to be set there.
void registerReopenHandler(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (reopenHandler != nil) {
            return;
        }
        reopenHandler = [[EXReopenHandler alloc] init];
        [[NSAppleEventManager sharedAppleEventManager]
            setEventHandler:reopenHandler
                andSelector:@selector(handleReopen:withReplyEvent:)
              forEventClass:kCoreEventClass
                 andEventID:kAEReopenApplication];
    });
}
