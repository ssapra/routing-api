package helpers_test

import (
	"errors"
	"syscall"
	"time"

	fake_db "github.com/cloudfoundry-incubator/routing-api/db/fakes"
	"github.com/cloudfoundry-incubator/routing-api/helpers"
	"github.com/cloudfoundry-incubator/routing-api/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Helpers", func() {
	Describe("RouteRegister", func() {
		var (
			routeRegister *helpers.RouteRegister
			database      *fake_db.FakeDB
			route         models.Route
			logger        *lagertest.TestLogger

			timeChan chan time.Time
			ticker   *time.Ticker
		)

		var process ifrit.Process

		BeforeEach(func() {
			route = models.Route{
				Route:   "i dont care",
				Port:    3000,
				IP:      "i dont care even more",
				LogGuid: "i care a little bit more now",
				TTL:     new(int),
			}
			*route.TTL = 120
			database = &fake_db.FakeDB{}
			logger = lagertest.NewTestLogger("event-handler-test")

			timeChan = make(chan time.Time)
			ticker = &time.Ticker{C: timeChan}

			routeRegister = helpers.NewRouteRegister(database, route, ticker, logger)
		})

		AfterEach(func() {
			process.Signal(syscall.SIGTERM)
		})

		JustBeforeEach(func() {
			process = ifrit.Invoke(routeRegister)
		})

		Context("registration", func() {
			It("registers the route for a routing api on init", func() {
				Eventually(database.SaveRouteCallCount).Should(Equal(1))
				Eventually(func() models.Route { return database.SaveRouteArgsForCall(0) }).Should(Equal(route))
			})

			It("registers on an interval", func() {
				timeChan <- time.Now()

				Eventually(database.SaveRouteCallCount).Should(Equal(2))
				Eventually(func() models.Route { return database.SaveRouteArgsForCall(1) }).Should(Equal(route))
				Eventually(logger.Logs).Should(HaveLen(0))
			})

			Context("when there are errors", func() {
				BeforeEach(func() {
					var counter int
					// Do not error during setup phase
					database.SaveRouteStub = func(route models.Route) error {
						if counter > 0 {
							return errors.New("beep boop, self destruct mode engaged")
						}
						counter++
						return nil
					}
				})

				It("logs the error for each attempt", func() {
					Consistently(func() int { return len(logger.Logs()) }).Should(Equal(0))

					timeChan <- time.Now()
					Eventually(func() int { return len(logger.Logs()) }).Should(Equal(1))
					Eventually(func() string {
						if len(logger.Logs()) > 0 {
							return logger.Logs()[0].Data["error"].(string)
						} else {
							return ""
						}
					}).Should(ContainSubstring("beep boop, self destruct mode engaged"))

					timeChan <- time.Now()
					Eventually(func() int { return len(logger.Logs()) }).Should(Equal(2))
				})

				Context("during startup", func() {
					BeforeEach(func() {
						database.SaveRouteReturns(errors.New("startup error"))
					})

					It("returns an error immediately", func() {
						var err error
						Expect(database.SaveRouteCallCount()).To(Equal(1))
						waitChan := process.Wait()
						Eventually(waitChan).Should(Receive(&err))
						Expect(err.Error()).To(ContainSubstring("startup error"))
					})
				})
			})
		})

		Context("unregistration", func() {
			It("unregisters the routing api when a SIGTERM is received", func() {
				process.Signal(syscall.SIGTERM)
				Eventually(database.DeleteRouteCallCount).Should(Equal(1))
				Eventually(func() models.Route {
					return database.DeleteRouteArgsForCall(0)
				}).Should(Equal(route))
			})
		})
	})

})
