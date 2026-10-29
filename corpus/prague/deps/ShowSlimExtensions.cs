using CommonFeedModels.Enums;

namespace Common.EventApi.Cache.Prague.Models;

public static class ShowSlimExtensions
{
    public static EventStatus GetCurrentStatus(this SlimShow show)
    {
        return show.EventStatusWatchDog ?? show.EventStatusManual ?? show.EventStatus;
    }

    public static QuoteStatus GetCurrentStatus(this SlimQuote quote)
    {
        return quote.StatusManual ?? quote.Status;
    }

    public static TicketTypeStatus GetCurrentStatus(this SlimTicketType ticketType)
    {
        return ticketType.StatusManual ?? ticketType.Status;
    }
}
